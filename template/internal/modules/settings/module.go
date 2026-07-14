// Package settings is the module that owns user preferences.
//
// Layout, which every module in this codebase repeats:
//
//	api/      the public contract. The only package others may import.
//	service/  the use cases. Private.
//	ui/       the screens. Private.
//	module.go the wiring: what this module provides, needs, and shows.
package settings

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/example/gtkapp/internal/app"
	settingsapi "github.com/example/gtkapp/internal/modules/settings/api"
	"github.com/example/gtkapp/internal/modules/settings/service"
	settingsui "github.com/example/gtkapp/internal/modules/settings/ui"
	"github.com/example/gtkapp/internal/platform/ui"
)

// Module is the settings feature.
type Module struct {
	store *service.Store
	ctx   context.Context
}

// New returns the module, ready to be mounted on the kernel.
func New() *Module { return &Module{} }

// ID namespaces the module's config key and database tables.
func (m *Module) ID() string { return "settings" }

// Register builds the store and publishes it under the api interface.
//
// Note what is published: the settingsapi.Store interface, not the
// *service.Store struct. A consumer resolving the interface compiles against the
// contract, so this module stays free to change how it stores things.
func (m *Module) Register(k *app.Kernel) error {
	store, err := service.New(k.Cfg, k.Bus)
	if err != nil {
		return err
	}
	m.store = store

	app.Provide[settingsapi.Store](k, store)
	return nil
}

// Start applies the saved theme, so the window opens in the scheme the user
// chose rather than flashing the default first.
func (m *Module) Start(ctx context.Context, k *app.Kernel) error {
	m.ctx = ctx

	theme := m.store.Get().Theme
	ui.OnMain(func() { settingsui.ApplyTheme(theme) })

	return nil
}

// Views contributes the preferences screen.
func (m *Module) Views() []app.View {
	return []app.View{{
		ID:    "settings.preferences",
		Title: "Settings",
		Icon:  "preferences-system-symbolic",
		// Settings belong at the bottom of the sidebar, under whatever the app
		// is actually for.
		Order: 900,
		Build: func(k *app.Kernel) gtk.Widgetter {
			return settingsui.NewPage(m.ctx, m.store, app.Use[ui.Toaster](k))
		},
	}}
}

// Compile-time proof that the module satisfies the optional interfaces it means
// to. Without these, dropping a method (or misspelling one) silently turns the
// feature off instead of failing the build.
var (
	_ app.Module        = (*Module)(nil)
	_ app.Starter       = (*Module)(nil)
	_ app.UIContributor = (*Module)(nil)
)
