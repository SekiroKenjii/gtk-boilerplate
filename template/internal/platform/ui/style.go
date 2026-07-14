package ui

import (
	_ "embed"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

//go:embed style.css
var styleCSS string

// LoadStyles installs the application stylesheet on the default display.
//
// The CSS is compiled into the binary rather than shipped as a GResource. A
// GResource would be the GNOME-idiomatic choice, but it means running
// glib-compile-resources before `go build` will even succeed, and a boilerplate
// that cannot be built with the Go toolchain alone is a boilerplate people work
// around.
//
// The priority is APPLICATION, which sits above libadwaita's own theme and below
// anything the user has put in ~/.config/gtk-4.0/gtk.css. Both halves matter: our
// styles win over the default theme, and a user who has themed their desktop
// still gets the last word.
//
// Call this once, after GTK is initialised and before the first window is shown.
func LoadStyles() {
	display := gdk.DisplayGetDefault()
	if display == nil {
		// No display means no styling to do, and shortly no window either.
		// The caller will fail on its own terms with a better message than a
		// nil dereference here.
		return
	}

	provider := gtk.NewCSSProvider()
	provider.LoadFromString(styleCSS)

	gtk.StyleContextAddProviderForDisplay(
		display,
		provider,
		uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION),
	)
}
