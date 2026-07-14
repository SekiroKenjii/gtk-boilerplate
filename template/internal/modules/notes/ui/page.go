// Package ui holds the notes module's screens. It is private to the module.
//
// Note what it imports: settingsapi, the settings module's contract. It does not
// import settings/service, which would reach past the boundary. This is the
// sanctioned way for one module to use another, and tools/archlint enforces it.
package ui

import (
	"context"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	notesapi "github.com/example/gtkapp/internal/modules/notes/api"
	settingsapi "github.com/example/gtkapp/internal/modules/settings/api"
	"github.com/example/gtkapp/internal/platform/ui"
)

// summaryRunes is how much of a note's body a sidebar row previews.
const summaryRunes = 60

// Page is the notes screen: a list on the left, an editor on the right.
type Page struct {
	ctx      context.Context
	notes    notesapi.Service
	settings settingsapi.Store
	toaster  ui.Toaster

	root    *gtk.Paned
	list    *gtk.ListBox
	stack   *gtk.Stack
	title   *gtk.Entry
	body    *gtk.TextView
	delete_ *gtk.Button

	loaded   []notesapi.Note
	selected int64

	// selecting guards the ListBox's row-selected handler while the list is
	// being rebuilt. Repopulating a ListBox fires selection changes for rows
	// that are on their way out, and acting on those would load a note the user
	// never clicked, or clobber the editor mid-edit.
	selecting bool
}

// NewPage builds the notes screen and kicks off the first load.
func NewPage(
	ctx context.Context,
	notes notesapi.Service,
	settings settingsapi.Store,
	toaster ui.Toaster,
) gtk.Widgetter {
	p := &Page{
		ctx:      ctx,
		notes:    notes,
		settings: settings,
		toaster:  toaster,
	}

	p.root = gtk.NewPaned(gtk.OrientationHorizontal)
	p.root.SetStartChild(p.buildList())
	p.root.SetEndChild(p.buildEditor())
	p.root.SetPosition(300)
	p.root.SetResizeStartChild(false)

	p.reload()
	return p.root
}

func (p *Page) buildList() gtk.Widgetter {
	p.list = gtk.NewListBox()
	p.list.SetSelectionMode(gtk.SelectionSingle)
	p.list.AddCSSClass("navigation-sidebar")
	p.list.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if p.selecting || row == nil {
			return
		}
		i := row.Index()
		if i < 0 || i >= len(p.loaded) {
			return
		}
		p.open(p.loaded[i])
	})

	scroll := gtk.NewScrolledWindow()
	scroll.SetVExpand(true)
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetChild(p.list)

	newButton := gtk.NewButtonFromIconName("list-add-symbolic")
	newButton.SetTooltipText("New note")
	newButton.ConnectClicked(p.create)

	p.delete_ = gtk.NewButtonFromIconName("user-trash-symbolic")
	p.delete_.SetTooltipText("Delete note")
	p.delete_.AddCSSClass("destructive-action")
	p.delete_.SetSensitive(false)
	p.delete_.ConnectClicked(p.confirmDelete)

	bar := gtk.NewActionBar()
	bar.PackStart(newButton)
	bar.PackEnd(p.delete_)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.Append(scroll)
	box.Append(bar)
	return box
}

func (p *Page) buildEditor() gtk.Widgetter {
	p.title = gtk.NewEntry()
	p.title.SetPlaceholderText("Title")
	p.title.AddCSSClass("title-2")

	p.body = gtk.NewTextView()
	p.body.SetWrapMode(gtk.WrapWord)
	p.body.SetTopMargin(8)
	p.body.SetBottomMargin(8)
	p.body.SetLeftMargin(4)
	p.body.SetRightMargin(4)

	bodyScroll := gtk.NewScrolledWindow()
	bodyScroll.SetVExpand(true)
	bodyScroll.SetChild(p.body)

	save := gtk.NewButtonWithLabel("Save")
	save.AddCSSClass("suggested-action")
	save.ConnectClicked(p.save)

	actions := gtk.NewBox(gtk.OrientationHorizontal, 6)
	actions.SetHAlign(gtk.AlignEnd)
	actions.Append(save)

	form := gtk.NewBox(gtk.OrientationVertical, 12)
	form.SetMarginTop(18)
	form.SetMarginBottom(18)
	form.SetMarginStart(18)
	form.SetMarginEnd(18)
	form.Append(p.title)
	form.Append(bodyScroll)
	form.Append(actions)

	clamp := adw.NewClamp()
	clamp.SetMaximumSize(720)
	clamp.SetChild(form)

	empty := adw.NewStatusPage()
	empty.SetIconName("document-edit-symbolic")
	empty.SetTitle("No note selected")
	empty.SetDescription("Pick a note from the list, or create one.")
	empty.AddCSSClass("app-empty-state")

	p.stack = gtk.NewStack()
	p.stack.SetHExpand(true)
	p.stack.AddNamed(empty, "empty")
	p.stack.AddNamed(clamp, "editor")
	p.stack.SetVisibleChildName("empty")
	return p.stack
}

// reload fetches the notes and redraws the list, preserving the selection when
// the selected note survived.
func (p *Page) reload() {
	ui.Async(p.ctx,
		func(ctx context.Context) ([]notesapi.Note, error) {
			return p.notes.List(ctx)
		},
		func(notes []notesapi.Note, err error) {
			if err != nil {
				p.toaster.ToastError(err)
				return
			}
			p.render(notes)
		},
	)
}

func (p *Page) render(notes []notesapi.Note) {
	p.selecting = true
	p.list.RemoveAll()
	p.loaded = notes

	selectedRow := -1
	for i, n := range notes {
		row := adw.NewActionRow()
		row.SetTitle(n.Title)
		row.SetSubtitle(summarize(n.Body))
		p.list.Append(row)

		if n.ID == p.selected {
			selectedRow = i
		}
	}
	p.selecting = false

	if selectedRow >= 0 {
		if row := p.list.RowAtIndex(selectedRow); row != nil {
			p.list.SelectRow(row)
		}
		return
	}

	// The selected note is gone: deleted, or this is the first load. Fall back
	// to the empty state rather than leaving a stale note in the editor.
	p.clearEditor()
}

func (p *Page) open(n notesapi.Note) {
	p.selected = n.ID
	p.title.SetText(n.Title)
	p.body.Buffer().SetText(n.Body)
	p.stack.SetVisibleChildName("editor")
	p.delete_.SetSensitive(true)
}

func (p *Page) clearEditor() {
	p.selected = 0
	p.title.SetText("")
	p.body.Buffer().SetText("")
	p.stack.SetVisibleChildName("empty")
	p.delete_.SetSensitive(false)
}

func (p *Page) create() {
	ui.Async(p.ctx,
		func(ctx context.Context) (notesapi.Note, error) {
			return p.notes.Create(ctx, "Untitled note", "")
		},
		func(n notesapi.Note, err error) {
			if err != nil {
				p.toaster.ToastError(err)
				return
			}
			// Select the new note before reloading, so render finds it and
			// opens it. The user asked for a note; they should land in it.
			p.selected = n.ID
			p.reload()
			p.title.GrabFocus()
		},
	)
}

func (p *Page) save() {
	if p.selected == 0 {
		return
	}

	id := p.selected
	title := p.title.Text()
	body := p.bodyText()

	ui.Async(p.ctx,
		func(ctx context.Context) (notesapi.Note, error) {
			return p.notes.Update(ctx, id, title, body)
		},
		func(_ notesapi.Note, err error) {
			if err != nil {
				// An empty title lands here, with a message written for the
				// user. Leave the editor exactly as it is so nothing they typed
				// is lost to a failed save.
				p.toaster.ToastError(err)
				return
			}
			p.toaster.Toast("Note saved")
			p.reload()
		},
	)
}

// confirmDelete asks first, if the user has asked to be asked.
//
// This is the cross-module dependency in the flesh: notes has no opinion about
// whether confirmations are wanted, and no access to how that preference is
// stored. It asks the settings contract.
func (p *Page) confirmDelete() {
	if p.selected == 0 {
		return
	}

	if !p.settings.Get().ConfirmDelete {
		p.doDelete(p.selected)
		return
	}

	title := p.title.Text()

	dialog := adw.NewAlertDialog(
		"Delete this note?",
		"“"+title+"” will be gone for good.",
	)
	dialog.AddResponse("cancel", "Cancel")
	dialog.AddResponse("delete", "Delete")
	dialog.SetResponseAppearance("delete", adw.ResponseDestructive)
	dialog.SetDefaultResponse("cancel")
	dialog.SetCloseResponse("cancel")

	id := p.selected
	dialog.ConnectResponse(func(response string) {
		if response == "delete" {
			p.doDelete(id)
		}
	})

	dialog.Present(p.root)
}

func (p *Page) doDelete(id int64) {
	ui.Do(p.ctx,
		func(ctx context.Context) error { return p.notes.Delete(ctx, id) },
		func(err error) {
			if err != nil {
				p.toaster.ToastError(err)
				return
			}
			p.clearEditor()
			p.reload()
			p.toaster.Toast("Note deleted")
		},
	)
}

func (p *Page) bodyText() string {
	buf := p.body.Buffer()
	start, end := buf.Bounds()
	return buf.Text(start, end, false)
}

// summarize renders a note's body as a single-line preview for a list row.
//
// Newlines are collapsed rather than honoured: a preview that kept them would
// give every row a different height, and the list would jump about as the user
// typed.
func summarize(body string) string {
	line := strings.Join(strings.Fields(body), " ")
	if line == "" {
		return "Empty note"
	}

	// Truncate by runes, not bytes. Slicing a multi-byte character in half
	// renders as a replacement glyph, which is immediately visible in any
	// language that is not English.
	runes := []rune(line)
	if len(runes) <= summaryRunes {
		return line
	}
	return string(runes[:summaryRunes]) + "…"
}
