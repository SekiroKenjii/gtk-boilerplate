// Package settingsapi is the settings module's public contract.
//
// This package is the module's entire outside surface. Other modules may import
// it; they may not import anything else under internal/modules/settings/, and
// tools/archlint fails the build if they try.
//
// The rule exists so that the settings module can rewrite its storage, its
// service, or its screens without breaking anyone, as long as this file keeps
// its promises. Keep the package free of implementation: interfaces, plain data,
// events. No GTK, no SQL, no dependencies on other modules.
package settingsapi

import "context"

// Theme is the colour scheme the user has chosen.
type Theme string

const (
	// ThemeSystem follows the desktop's light/dark preference. It is the
	// default, and for most users it should stay that way.
	ThemeSystem Theme = "system"
	ThemeLight  Theme = "light"
	ThemeDark   Theme = "dark"
)

// Valid reports whether t is a theme the application knows about. A config file
// written by a newer version, or edited by hand, can contain anything.
func (t Theme) Valid() bool {
	switch t {
	case ThemeSystem, ThemeLight, ThemeDark:
		return true
	}
	return false
}

// Settings is the full set of user preferences, as a value.
//
// It is passed and returned by copy, so a caller cannot mutate the store's state
// by holding on to what it was given.
type Settings struct {
	Theme Theme `json:"theme"`

	// ConfirmDelete asks the user before destroying anything. Other modules
	// read this to decide whether to put a dialog in the way; see the notes
	// module's delete flow.
	ConfirmDelete bool `json:"confirm_delete"`
}

// Defaults are the settings a fresh installation starts with.
func Defaults() Settings {
	return Settings{
		Theme: ThemeSystem,
		// Default to asking. A user who finds the prompt tedious can turn it
		// off in one click; a user who loses work to a misclick cannot get it
		// back.
		ConfirmDelete: true,
	}
}

// Store reads and writes user preferences.
type Store interface {
	// Get returns the current settings. The values are held in memory, so it
	// is cheap: callers are free to call it on every interaction rather than
	// caching a copy that will go stale.
	Get() Settings

	// Update persists s and publishes a Changed event. Passing an invalid
	// Theme is an error, not a silent fallback.
	Update(ctx context.Context, s Settings) error
}

// Changed is published after settings are successfully persisted.
//
// Subscribe to it when your module has to react to a preference: repaint,
// reload, re-render. Do not subscribe merely to read a value. Call Store.Get at
// the point you need it, and you cannot be out of date.
type Changed struct {
	Settings Settings
}
