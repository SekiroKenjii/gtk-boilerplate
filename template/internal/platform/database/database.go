// Package database wraps the application's SQLite store.
//
// The driver is modernc.org/sqlite, a pure-Go translation of SQLite. Its C
// competitor is faster, but this application already links GTK through cgo and
// adding a second C dependency buys a cross-compilation headache for a database
// that will hold, at most, one user's data.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// DB is the application's single SQLite handle.
//
// Every module shares it. They stay out of each other's way by convention, not
// by isolation: a module may only touch tables prefixed with its own ID. The
// architecture linter cannot check that for you. Code review has to.
type DB struct {
	*sql.DB
	path string
}

// Migration is one forward schema change owned by a module.
type Migration struct {
	Name string
	SQL  string
}

// Open connects to the SQLite file at path, creating the parent directory when
// it does not exist. Pass ":memory:" for tests.
func Open(ctx context.Context, path string) (*DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
	}

	// Pragmas, in the order they matter:
	//
	//   _pragma=journal_mode(WAL)  a reader never blocks the writer, so a slow
	//                              query on a background goroutine cannot freeze
	//                              the goroutine the UI is waiting on.
	//   _pragma=foreign_keys(1)    SQLite ignores foreign keys unless asked. The
	//                              default is a footgun; turn it off and a
	//                              cascade delete silently does nothing.
	//   _pragma=busy_timeout(5000) wait instead of returning SQLITE_BUSY the
	//                              instant two writers overlap.
	dsn := path
	if path != ":memory:" {
		dsn = fmt.Sprintf(
			"file:%s?_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)",
			path,
		)
	}

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// SQLite serialises writes anyway, and a desktop app has no throughput
	// problem worth solving. One connection removes a whole class of
	// "database is locked" races for free.
	sqlDB.SetMaxOpenConns(1)

	if err := sqlDB.PingContext(ctx); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}

	db := &DB{DB: sqlDB, path: path}
	if err := db.initMigrationTable(ctx); err != nil {
		sqlDB.Close()
		return nil, err
	}
	return db, nil
}

// Path is the file the database lives in, or ":memory:".
func (db *DB) Path() string { return db.path }

func (db *DB) initMigrationTable(ctx context.Context) error {
	const stmt = `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			module     TEXT NOT NULL,
			name       TEXT NOT NULL,
			applied_at TEXT NOT NULL DEFAULT (datetime('now')),
			PRIMARY KEY (module, name)
		) STRICT;`
	if _, err := db.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}
	return nil
}

// Migrate applies the migrations a module has not run yet, in order.
//
// Each migration commits in its own transaction. That is deliberate: if the
// third of five fails, the first two stay applied and recorded, and the next
// boot resumes at the third instead of replaying work that already succeeded.
//
// Migrations are keyed by (module, name), so two modules are free to use the
// same migration name, and renaming a shipped migration makes it run a second
// time. That is why the Migration doc comment tells you not to.
func (db *DB) Migrate(ctx context.Context, module string, migrations []Migration) error {
	applied, err := db.appliedMigrations(ctx, module)
	if err != nil {
		return err
	}

	for _, m := range migrations {
		if _, done := applied[m.Name]; done {
			continue
		}

		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin migration %s/%s: %w", module, m.Name, err)
		}

		if _, err := tx.ExecContext(ctx, m.SQL); err != nil {
			tx.Rollback()
			return fmt.Errorf("apply migration %s/%s: %w", module, m.Name, err)
		}

		const record = `INSERT INTO schema_migrations (module, name) VALUES (?, ?);`
		if _, err := tx.ExecContext(ctx, record, module, m.Name); err != nil {
			tx.Rollback()
			return fmt.Errorf("record migration %s/%s: %w", module, m.Name, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s/%s: %w", module, m.Name, err)
		}
	}

	return nil
}

func (db *DB) appliedMigrations(ctx context.Context, module string) (map[string]struct{}, error) {
	const q = `SELECT name FROM schema_migrations WHERE module = ?;`

	rows, err := db.QueryContext(ctx, q, module)
	if err != nil {
		return nil, fmt.Errorf("read applied migrations: %w", err)
	}
	defer rows.Close()

	applied := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("scan applied migration: %w", err)
		}
		applied[name] = struct{}{}
	}
	return applied, rows.Err()
}
