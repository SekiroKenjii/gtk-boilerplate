// Package dashboard is a read-only module built entirely out of other modules'
// contracts.
//
// It exists to show the two ways modules are allowed to depend on each other,
// side by side:
//
//	synchronous   it calls notesapi.Service to ask how many notes there are.
//	asynchronous  it subscribes to notesapi.Created and notesapi.Deleted so the
//	              number updates itself when something changes elsewhere.
//
// It owns no tables, provides no service, and imports no other module's
// internals. Deleting this directory and its line in cmd/app/main.go removes the
// feature completely, which is the property the whole architecture is for.
package dashboard

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/example/gtkapp/internal/app"
	dashboardui "github.com/example/gtkapp/internal/modules/dashboard/ui"
	notesapi "github.com/example/gtkapp/internal/modules/notes/api"
)

// Module is the dashboard feature.
type Module struct {
	ctx  context.Context
	page *dashboardui.Page
}

// New returns the module, ready to be mounted on the kernel.
func New() *Module { return &Module{} }

// ID identifies the module.
func (m *Module) ID() string { return "dashboard" }

// Register subscribes to the notes events.
//
// Subscribing in Register, rather than Start, means no event can slip past
// while the module is still coming up. Resolving a service here would be the
// mistake; subscribing is not, because the bus does not care whether the
// publisher exists yet.
func (m *Module) Register(k *app.Kernel) error {
	app.Subscribe(k, func(ctx context.Context, e notesapi.Created) {
		m.refresh(k)
	})
	app.Subscribe(k, func(ctx context.Context, e notesapi.Deleted) {
		m.refresh(k)
	})
	return nil
}

// Start records the application context.
func (m *Module) Start(ctx context.Context, k *app.Kernel) error {
	m.ctx = ctx
	return nil
}

// refresh asks the page to recount, if the page has ever been built.
//
// Event handlers run on whichever goroutine published, which is not the GTK main
// thread. The page is responsible for getting back there, and it does.
func (m *Module) refresh(k *app.Kernel) {
	if m.page == nil {
		// Nobody has opened the dashboard, so there is nothing to update. The
		// count is computed on first open anyway.
		return
	}
	m.page.Refresh()
}

// Views contributes the overview screen.
func (m *Module) Views() []app.View {
	return []app.View{{
		ID:    "dashboard.overview",
		Title: "Overview",
		Icon:  "view-grid-symbolic",
		// First in the sidebar: it is the screen that makes sense to land on.
		Order: 10,
		Build: func(k *app.Kernel) gtk.Widgetter {
			m.page = dashboardui.NewPage(m.ctx, app.Use[notesapi.Service](k))
			return m.page.Widget()
		},
	}}
}

var (
	_ app.Module        = (*Module)(nil)
	_ app.Starter       = (*Module)(nil)
	_ app.UIContributor = (*Module)(nil)
)
