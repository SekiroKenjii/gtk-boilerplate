package app

// The scaffolding tool rewrites the four values below when it generates a
// project. They are the only place the application's identity is spelled out,
// so keep every other file referring back to these constants.
const (
	// ID is the D-Bus / desktop-entry application ID. GTK derives the default
	// window icon and the GSettings path from it, and the desktop file must be
	// named after it, so it has to stay a valid reverse-DNS name.
	ID = "com.example.GtkApp"

	// Name is what the user sees: window titles, the About dialog, toasts.
	Name = "GtkApp"

	// Binary is the executable name, also used for the XDG state and cache
	// directories.
	Binary = "gtkapp"

	// Version is overwritten at link time by the Makefile. The value here is
	// the fallback for `go run`.
	Version = "0.1.0-dev"
)

// Build metadata, injected with -ldflags. See the Makefile.
var (
	Commit = "unknown"
	Date   = "unknown"
)
