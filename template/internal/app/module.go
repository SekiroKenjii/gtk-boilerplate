package app

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// Module is one bounded context inside the monolith. Everything a feature owns
// (its domain types, its database tables, its services, its screens) lives
// under internal/modules/<id>/ and is reachable from the outside only through
// that module's api package.
//
// A module goes through the lifecycle below, in order, with every module
// finishing a stage before any module starts the next one. That is what lets a
// module resolve another module's service in Start without caring which order
// the two were registered in.
//
//  1. Register: publish services, subscribe to events. Do not resolve
//     anything here, because the modules registered after you have not run
//     yet.
//  2. Migrate: create or upgrade the tables you own. Optional.
//  3. Start: resolve dependencies, open connections, spawn goroutines.
//     Optional.
//  4. Views: hand the shell the screens you contribute. Optional.
//  5. Stop: release what Start acquired, in reverse registration order.
//     Optional.
type Module interface {
	// ID is a stable, lowercase identifier. It namespaces the module's
	// database tables, its config keys and its GAction group, so changing it
	// later is a migration, not a rename.
	ID() string

	// Register publishes this module's services into the kernel and wires up
	// event subscriptions. It must not resolve services from other modules.
	Register(k *Kernel) error
}

// Migrator is implemented by modules that own database tables. Migrations run
// once, in registration order, inside a single transaction per module.
type Migrator interface {
	// Migrations returns the module's ordered, append-only migration list. Once
	// a migration has shipped, never edit it. Add another one instead.
	Migrations() []Migration
}

// Migration is a single forward schema change. Name has to be unique within the
// module and must never change once released; it is the primary key the kernel
// uses to decide what has already been applied.
type Migration struct {
	Name string
	SQL  string
}

// Starter is implemented by modules that need to do work once every module has
// registered. This is the earliest point at which Use[T] is safe.
type Starter interface {
	Start(ctx context.Context, k *Kernel) error
}

// Stopper is implemented by modules holding resources that outlive Start.
// Stop runs in reverse registration order and is given a context with a
// deadline, so respect cancellation rather than blocking shutdown.
type Stopper interface {
	Stop(ctx context.Context) error
}

// UIContributor is implemented by modules with a user interface. The shell
// collects every view, sorts them, and renders one sidebar entry per view.
type UIContributor interface {
	Views() []View
}

// View is a screen a module contributes to the shell's sidebar.
type View struct {
	// ID is unique across the whole application; the shell uses it as the
	// navigation stack's page name. Convention is "<module>.<screen>".
	ID string

	// Title and Icon label the sidebar row. Icon is a symbolic icon name from
	// the active icon theme, e.g. "document-edit-symbolic".
	Title string
	Icon  string

	// Order sorts the sidebar. Lower comes first; ties break on Title.
	Order int

	// Build constructs the screen. The shell calls it lazily, the first time
	// the user navigates to the view, and caches the result, so a module that
	// is compiled in but never opened costs nothing but its Register.
	//
	// Build always runs on the GTK main thread.
	Build func(k *Kernel) gtk.Widgetter
}
