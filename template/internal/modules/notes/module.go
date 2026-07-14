// Package notes is the example feature module: a full vertical slice from
// database to screen.
//
// Copy this directory to start a new module. What each layer is for:
//
//	api/      the contract. Interfaces, DTOs, events. The only package other
//	          modules may import, and the only one that may not import them.
//	domain/   entities and the rules they enforce. Knows nothing about SQL or
//	          GTK, which is what makes it testable in isolation.
//	store/    persistence. Owns every table prefixed notes_.
//	service/  use cases. Orchestrates domain and store, publishes events.
//	ui/       screens.
//	module.go the wiring below.
package notes

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/example/gtkapp/internal/app"
	notesapi "github.com/example/gtkapp/internal/modules/notes/api"
	"github.com/example/gtkapp/internal/modules/notes/service"
	"github.com/example/gtkapp/internal/modules/notes/store"
	notesui "github.com/example/gtkapp/internal/modules/notes/ui"
	settingsapi "github.com/example/gtkapp/internal/modules/settings/api"
	"github.com/example/gtkapp/internal/platform/ui"
)

// Module is the notes feature.
type Module struct {
	svc *service.Service
	ctx context.Context
}

// New returns the module, ready to be mounted on the kernel.
func New() *Module { return &Module{} }

// ID namespaces the module's tables (notes_*) and config key.
func (m *Module) ID() string { return "notes" }

// Migrations hands the kernel this module's schema. The kernel applies whatever
// has not run yet, in order, before any module starts.
func (m *Module) Migrations() []app.Migration {
	out := make([]app.Migration, 0, len(store.Migrations()))
	for _, mg := range store.Migrations() {
		out = append(out, app.Migration{Name: mg.Name, SQL: mg.SQL})
	}
	return out
}

// Register builds the service and publishes it under the api interface.
func (m *Module) Register(k *app.Kernel) error {
	m.svc = service.New(store.New(k.DB), k.Bus, nil)

	app.Provide[notesapi.Service](k, m.svc)
	return nil
}

// Start records the application context. Nothing else to do: the module has no
// background work, and its dependency on settings is resolved lazily, in Views,
// because a screen that is never opened should not force the settings module to
// exist.
func (m *Module) Start(ctx context.Context, k *app.Kernel) error {
	m.ctx = ctx
	return nil
}

// Views contributes the notes screen.
func (m *Module) Views() []app.View {
	return []app.View{{
		ID:    "notes.list",
		Title: "Notes",
		Icon:  "document-edit-symbolic",
		Order: 100,
		Build: func(k *app.Kernel) gtk.Widgetter {
			// Resolving here, rather than in Register, is what makes module
			// order irrelevant: by the time a view is built, every module has
			// registered and every contract is on the kernel.
			return notesui.NewPage(
				m.ctx,
				m.svc,
				app.Use[settingsapi.Store](k),
				app.Use[ui.Toaster](k),
			)
		},
	}}
}

var (
	_ app.Module        = (*Module)(nil)
	_ app.Migrator      = (*Module)(nil)
	_ app.Starter       = (*Module)(nil)
	_ app.UIContributor = (*Module)(nil)
)
