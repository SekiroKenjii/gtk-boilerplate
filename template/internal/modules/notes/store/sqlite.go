// Package store persists notes. It is private to the notes module.
//
// Every table it touches is prefixed notes_. The database is shared between
// modules, so the prefix is the only thing keeping one module's schema out of
// another's. Treat it as a hard rule, not a naming preference.
package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/example/gtkapp/internal/modules/notes/domain"
	"github.com/example/gtkapp/internal/platform/database"
	"github.com/example/gtkapp/internal/platform/errs"
)

// Migrations is the module's append-only schema history.
//
// Never edit a migration that has shipped: it is keyed by name, so a released
// migration whose SQL changed will not re-run on machines that already have it,
// and the schema silently diverges between users. Add a new one instead.
func Migrations() []database.Migration {
	return []database.Migration{
		{
			Name: "0001_create_notes",
			SQL: `
				CREATE TABLE notes_note (
					id         INTEGER PRIMARY KEY AUTOINCREMENT,
					title      TEXT NOT NULL,
					body       TEXT NOT NULL DEFAULT '',
					created_at TEXT NOT NULL,
					updated_at TEXT NOT NULL
				) STRICT;

				CREATE INDEX notes_note_updated_at ON notes_note (updated_at DESC);
			`,
		},
	}
}

// SQLite stores notes in the shared database.
type SQLite struct {
	db *database.DB
}

// New returns a store backed by db.
func New(db *database.DB) *SQLite { return &SQLite{db: db} }

// List returns every note, most recently updated first.
func (s *SQLite) List(ctx context.Context) ([]domain.Note, error) {
	const q = `
		SELECT id, title, body, created_at, updated_at
		FROM notes_note
		ORDER BY updated_at DESC, id DESC;`

	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, errs.Internalf(err, "list notes")
	}
	defer rows.Close()

	var notes []domain.Note
	for rows.Next() {
		n, err := scanNote(rows)
		if err != nil {
			return nil, err
		}
		notes = append(notes, n)
	}
	if err := rows.Err(); err != nil {
		return nil, errs.Internalf(err, "list notes")
	}
	return notes, nil
}

// Get returns the note with the given ID.
func (s *SQLite) Get(ctx context.Context, id int64) (domain.Note, error) {
	const q = `
		SELECT id, title, body, created_at, updated_at
		FROM notes_note
		WHERE id = ?;`

	n, err := scanNote(s.db.QueryRowContext(ctx, q, id))
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Note{}, errs.NotFoundf("No note with ID %d.", id)
	}
	if err != nil {
		return domain.Note{}, err
	}
	return n, nil
}

// Insert stores n and returns it with its assigned ID.
func (s *SQLite) Insert(ctx context.Context, n domain.Note) (domain.Note, error) {
	const q = `
		INSERT INTO notes_note (title, body, created_at, updated_at)
		VALUES (?, ?, ?, ?);`

	res, err := s.db.ExecContext(ctx, q,
		n.Title, n.Body, formatTime(n.CreatedAt), formatTime(n.UpdatedAt),
	)
	if err != nil {
		return domain.Note{}, errs.Internalf(err, "insert note")
	}

	id, err := res.LastInsertId()
	if err != nil {
		return domain.Note{}, errs.Internalf(err, "read new note ID")
	}

	n.ID = id
	return n, nil
}

// Update rewrites an existing note.
func (s *SQLite) Update(ctx context.Context, n domain.Note) error {
	const q = `
		UPDATE notes_note
		SET title = ?, body = ?, updated_at = ?
		WHERE id = ?;`

	res, err := s.db.ExecContext(ctx, q, n.Title, n.Body, formatTime(n.UpdatedAt), n.ID)
	if err != nil {
		return errs.Internalf(err, "update note")
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return errs.Internalf(err, "update note")
	}
	if affected == 0 {
		return errs.NotFoundf("No note with ID %d.", n.ID)
	}
	return nil
}

// Delete removes a note. Deleting one that does not exist is not an error: the
// caller asked for it to be gone, and afterwards it is.
func (s *SQLite) Delete(ctx context.Context, id int64) error {
	const q = `DELETE FROM notes_note WHERE id = ?;`

	if _, err := s.db.ExecContext(ctx, q, id); err != nil {
		return errs.Internalf(err, "delete note")
	}
	return nil
}

// Count returns the number of stored notes.
func (s *SQLite) Count(ctx context.Context) (int, error) {
	const q = `SELECT COUNT(*) FROM notes_note;`

	var n int
	if err := s.db.QueryRowContext(ctx, q).Scan(&n); err != nil {
		return 0, errs.Internalf(err, "count notes")
	}
	return n, nil
}

// scanner is what *sql.Row and *sql.Rows have in common, so one scan function
// serves both the single-row and the many-row queries.
type scanner interface {
	Scan(dest ...any) error
}

func scanNote(sc scanner) (domain.Note, error) {
	var (
		n                    domain.Note
		createdAt, updatedAt string
	)

	if err := sc.Scan(&n.ID, &n.Title, &n.Body, &createdAt, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Hand this back untouched: only the caller knows which ID was
			// being looked for, so only the caller can write a useful message.
			return domain.Note{}, err
		}
		return domain.Note{}, errs.Internalf(err, "scan note")
	}

	var err error
	if n.CreatedAt, err = parseTime(createdAt); err != nil {
		return domain.Note{}, err
	}
	if n.UpdatedAt, err = parseTime(updatedAt); err != nil {
		return domain.Note{}, err
	}
	return n, nil
}

// SQLite has no timestamp type, so times are stored as RFC3339 strings in UTC.
// UTC matters: the strings sort lexicographically, which is what makes
// "ORDER BY updated_at" mean what it says, and a user who travels through a
// timezone would otherwise scramble their own note order.
func formatTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, errs.Internalf(err, "parse stored timestamp %q", s)
	}
	return t.Local(), nil
}
