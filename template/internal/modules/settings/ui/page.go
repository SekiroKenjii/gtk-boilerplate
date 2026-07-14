// Package ui holds the settings module's screens. It is private to the module.
package ui

import (
	"context"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	settingsapi "github.com/example/gtkapp/internal/modules/settings/api"
	"github.com/example/gtkapp/internal/platform/ui"
)

// themeOrder fixes the mapping between the combo row's integer selection and the
// Theme values. The combo hands back a position, so this slice is the source of
// truth, not the order the labels happen to appear in below.
var themeOrder = []settingsapi.Theme{
	settingsapi.ThemeSystem,
	settingsapi.ThemeLight,
	settingsapi.ThemeDark,
}

// ApplyTheme pushes t onto libadwaita's style manager.
//
// "System" hands control back to the desktop, so the app follows the user's
// night-light schedule instead of pinning itself to whatever they last picked.
// Light and dark are Force, not Prefer: the user asked for a specific scheme, and
// Prefer would let the system override them, which is the opposite of what the
// setting says.
//
// Must run on the GTK main thread.
func ApplyTheme(t settingsapi.Theme) {
	scheme := adw.ColorSchemeDefault
	switch t {
	case settingsapi.ThemeLight:
		scheme = adw.ColorSchemeForceLight
	case settingsapi.ThemeDark:
		scheme = adw.ColorSchemeForceDark
	}
	adw.StyleManagerGetDefault().SetColorScheme(scheme)
}

// NewPage builds the preferences screen.
func NewPage(ctx context.Context, store settingsapi.Store, toaster ui.Toaster) gtk.Widgetter {
	page := adw.NewPreferencesPage()
	page.SetTitle("Settings")
	page.SetIconName("preferences-system-symbolic")

	current := store.Get()

	appearance := adw.NewPreferencesGroup()
	appearance.SetTitle("Appearance")

	themeRow := adw.NewComboRow()
	themeRow.SetTitle("Theme")
	themeRow.SetSubtitle("System follows your desktop's light and dark preference")
	themeRow.SetModel(gtk.NewStringList([]string{"System", "Light", "Dark"}))
	themeRow.SetSelected(uint(indexOfTheme(current.Theme)))
	appearance.Add(themeRow)

	behaviour := adw.NewPreferencesGroup()
	behaviour.SetTitle("Behaviour")

	confirmRow := adw.NewSwitchRow()
	confirmRow.SetTitle("Confirm before deleting")
	confirmRow.SetSubtitle("Ask for confirmation when destroying something")
	confirmRow.SetActive(current.ConfirmDelete)
	behaviour.Add(confirmRow)

	page.Add(appearance)
	page.Add(behaviour)

	// Both rows write through the same helper, so the two settings can never
	// disagree about what the other one is: we always persist the whole struct,
	// read fresh from the widgets.
	save := func() {
		next := settingsapi.Settings{
			Theme:         themeOrder[clampIndex(int(themeRow.Selected()))],
			ConfirmDelete: confirmRow.Active(),
		}

		// Repaint immediately rather than waiting for the write to land. The
		// theme is the one setting whose effect the user is staring at, and a
		// visible lag between flipping it and seeing it reads as a broken app.
		ApplyTheme(next.Theme)

		ui.Do(ctx,
			func(ctx context.Context) error { return store.Update(ctx, next) },
			func(err error) {
				if err != nil {
					// The write failed, so the theme we just painted is a lie.
					// Put the old one back.
					ApplyTheme(store.Get().Theme)
					toaster.ToastError(err)
				}
			},
		)
	}

	themeRow.NotifyProperty("selected", save)
	confirmRow.NotifyProperty("active", save)

	return page
}

func indexOfTheme(t settingsapi.Theme) int {
	for i, candidate := range themeOrder {
		if candidate == t {
			return i
		}
	}
	return 0
}

func clampIndex(i int) int {
	if i < 0 || i >= len(themeOrder) {
		return 0
	}
	return i
}
