package ui

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"

	"github.com/example/gtkapp/internal/platform/errs"
)

// Toaster shows transient messages in the window's toast overlay.
//
// It is an interface, not the overlay itself, so a module can report "note
// saved" without a handle on the shell's widget tree, and so a service test can
// pass a no-op instead of standing up GTK.
type Toaster interface {
	// Toast shows a short confirmation.
	Toast(msg string)

	// ToastError shows err in terms the user can act on. Internal errors
	// collapse to a generic message; see errs.UserMessage for why.
	ToastError(err error)
}

// NewToaster wraps an Adwaita toast overlay.
//
// Both methods must be called on the GTK main thread. A module handling an event
// from the bus is usually not on it, so wrap the call in OnMain.
func NewToaster(overlay *adw.ToastOverlay) Toaster {
	return &overlayToaster{overlay: overlay}
}

type overlayToaster struct {
	overlay *adw.ToastOverlay
}

func (t *overlayToaster) Toast(msg string) {
	toast := adw.NewToast(msg)
	toast.SetTimeout(3)
	t.overlay.AddToast(toast)
}

func (t *overlayToaster) ToastError(err error) {
	if err == nil {
		return
	}
	toast := adw.NewToast(errs.UserMessage(err))
	// Errors linger: three seconds is enough to register a success, not enough
	// to read a failure and decide what to do about it.
	toast.SetTimeout(6)
	t.overlay.AddToast(toast)
}

// NopToaster discards everything. Use it in tests.
type NopToaster struct{}

func (NopToaster) Toast(string)     {}
func (NopToaster) ToastError(error) {}
