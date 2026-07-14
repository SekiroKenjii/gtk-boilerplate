// Package ui holds the dashboard module's screen. It is private to the module.
package ui

import (
	"context"
	"fmt"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	notesapi "github.com/example/gtkapp/internal/modules/notes/api"
	"github.com/example/gtkapp/internal/platform/ui"
)

// Page shows a live count of the notes in the application.
type Page struct {
	ctx   context.Context
	notes notesapi.Service

	status *adw.StatusPage
	root   gtk.Widgetter
}

// NewPage builds the overview screen and loads the first count.
func NewPage(ctx context.Context, notes notesapi.Service) *Page {
	p := &Page{ctx: ctx, notes: notes}

	p.status = adw.NewStatusPage()
	p.status.SetIconName("view-grid-symbolic")
	p.status.SetTitle("Overview")
	p.status.SetDescription("Counting…")

	clamp := adw.NewClamp()
	clamp.SetMaximumSize(600)
	clamp.SetChild(p.status)
	p.root = clamp

	p.Refresh()
	return p
}

// Widget is the screen, for the module to hand to the shell.
func (p *Page) Widget() gtk.Widgetter { return p.root }

// Refresh recounts the notes and redraws.
//
// It is safe to call from any goroutine: the count runs on a background one, and
// ui.Async brings the result back to the GTK main thread before it touches the
// widget. That matters here because the callers are event handlers, which run
// wherever the publisher happened to be.
func (p *Page) Refresh() {
	ui.Async(p.ctx,
		func(ctx context.Context) (int, error) {
			return p.notes.Count(ctx)
		},
		func(count int, err error) {
			if err != nil {
				p.status.SetDescription("Could not read the notes.")
				return
			}
			p.status.SetDescription(pluralize(count))
		},
	)
}

func pluralize(n int) string {
	switch n {
	case 0:
		return "No notes yet. Create one from the Notes screen."
	case 1:
		return "1 note."
	default:
		return fmt.Sprintf("%d notes.", n)
	}
}
