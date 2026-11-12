// Package wizardui is the graphical project setup: a libadwaita window that
// asks what you are building and writes it out.
//
// It is itself a GTK4 application, which is the point. The first thing the
// boilerplate does is prove its own stack works on your machine: if the wizard
// draws, the app you are about to generate will draw too.
package wizardui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/sekiroKenjii/gtk-boilerplate/internal/scaffold"
)

// Options configures the wizard.
type Options struct {
	// TemplateDir is the template to generate from.
	TemplateDir string

	// Defaults pre-fills the form. Anything the user passed on the command line
	// lands here.
	Defaults scaffold.Project
}

type wizard struct {
	opts Options
	app  *adw.Application
	win  *adw.ApplicationWindow
	nav  *adw.NavigationView

	name   *adw.EntryRow
	appID  *adw.EntryRow
	binary *adw.EntryRow
	module *adw.EntryRow
	author *adw.EntryRow
	dir    *adw.ActionRow

	notes     *adw.SwitchRow
	dashboard *adw.SwitchRow
	gitInit   *adw.SwitchRow
	tidy      *adw.SwitchRow

	banner *adw.Banner
	create *gtk.Button

	destination string

	// derived tracks whether the ID, binary and module are still the ones the
	// wizard guessed from the name. Once the user edits one by hand, the wizard
	// stops overwriting it. Silently undoing what somebody just typed is the
	// fastest way to make a form feel hostile.
	derived struct {
		appID  bool
		binary bool
		module bool
	}
}

// Run opens the wizard and blocks until it closes. It returns the process exit
// code.
func Run(opts Options) int {
	w := &wizard{opts: opts}
	w.derived.appID = true
	w.derived.binary = true
	w.derived.module = true

	w.app = adw.NewApplication("io.github.SekiroKenjii.Joinery", gio.ApplicationFlagsNone)
	w.app.ConnectActivate(func() {
		if w.win != nil {
			w.win.Present()
			return
		}
		w.build()
		w.win.Present()
	})

	return w.app.Run(nil)
}

func (w *wizard) build() {
	w.win = adw.NewApplicationWindow(&w.app.Application)
	w.win.SetTitle("Joinery")
	w.win.SetDefaultSize(680, 820)

	w.nav = adw.NewNavigationView()
	w.nav.Add(adw.NewNavigationPage(w.formPage(), "Configure"))

	w.win.SetContent(w.nav)
}

func (w *wizard) formPage() gtk.Widgetter {
	page := adw.NewPreferencesPage()

	// --- Application -------------------------------------------------------

	appGroup := adw.NewPreferencesGroup()
	appGroup.SetTitle("Application")
	appGroup.SetDescription("How the app introduces itself to the desktop")

	w.name = adw.NewEntryRow()
	w.name.SetTitle("Name")
	w.name.SetText(orDefault(w.opts.Defaults.Name, "My App"))

	w.appID = adw.NewEntryRow()
	w.appID.SetTitle("Application ID")

	w.binary = adw.NewEntryRow()
	w.binary.SetTitle("Executable name")

	appGroup.Add(w.name)
	appGroup.Add(w.appID)
	appGroup.Add(w.binary)

	// --- Project -----------------------------------------------------------

	projectGroup := adw.NewPreferencesGroup()
	projectGroup.SetTitle("Project")

	w.module = adw.NewEntryRow()
	w.module.SetTitle("Go module path")

	w.author = adw.NewEntryRow()
	w.author.SetTitle("Author")
	w.author.SetText(orDefault(w.opts.Defaults.Author, guessAuthor()))

	w.destination = orDefault(w.opts.Defaults.Dir, defaultDestination())

	w.dir = adw.NewActionRow()
	w.dir.SetTitle("Destination")
	w.dir.SetSubtitle(w.destination)

	browse := gtk.NewButtonWithLabel("Choose…")
	browse.SetVAlign(gtk.AlignCenter)
	browse.ConnectClicked(w.chooseDestination)
	w.dir.AddSuffix(browse)

	projectGroup.Add(w.module)
	projectGroup.Add(w.author)
	projectGroup.Add(w.dir)

	// --- Modules -----------------------------------------------------------

	modulesGroup := adw.NewPreferencesGroup()
	modulesGroup.SetTitle("Example modules")
	modulesGroup.SetDescription(
		"Working modules that demonstrate the architecture. Keep them to learn the " +
			"patterns, or start clean and write your own. Settings is always included: " +
			"the shell's theme depends on it.")

	w.notes = adw.NewSwitchRow()
	w.notes.SetTitle("Notes")
	w.notes.SetSubtitle("A full vertical slice: domain, store, service, screens")
	w.notes.SetActive(true)

	w.dashboard = adw.NewSwitchRow()
	w.dashboard.SetTitle("Dashboard")
	w.dashboard.SetSubtitle("Reads Notes through its contract and reacts to its events")
	w.dashboard.SetActive(true)

	// The dashboard is built entirely out of the notes contract, so it cannot
	// outlive it. Rather than let the user pick a combination that will not
	// compile, turn the switch off and say why.
	w.notes.NotifyProperty("active", func() {
		if !w.notes.Active() {
			w.dashboard.SetActive(false)
			w.dashboard.SetSensitive(false)
			w.dashboard.SetSubtitle("Needs the Notes module: it reads notes through notesapi")
		} else {
			w.dashboard.SetSensitive(true)
			w.dashboard.SetSubtitle("Reads Notes through its contract and reacts to its events")
		}
	})

	modulesGroup.Add(w.notes)
	modulesGroup.Add(w.dashboard)

	// --- After creating ----------------------------------------------------

	afterGroup := adw.NewPreferencesGroup()
	afterGroup.SetTitle("After creating")

	w.gitInit = adw.NewSwitchRow()
	w.gitInit.SetTitle("Initialise a git repository")
	w.gitInit.SetActive(true)

	w.tidy = adw.NewSwitchRow()
	w.tidy.SetTitle("Run go mod tidy")
	w.tidy.SetSubtitle("Downloads the dependencies. The first one pulls all of GTK's bindings, so give it a minute.")
	w.tidy.SetActive(true)

	afterGroup.Add(w.gitInit)
	afterGroup.Add(w.tidy)

	page.Add(appGroup)
	page.Add(projectGroup)
	page.Add(modulesGroup)
	page.Add(afterGroup)

	// Derive the ID, binary and module from the name until the user says
	// otherwise.
	w.name.NotifyProperty("text", w.onNameChanged)
	w.appID.NotifyProperty("text", func() { w.derived.appID = false })
	w.binary.NotifyProperty("text", func() { w.derived.binary = false })
	w.module.NotifyProperty("text", func() { w.derived.module = false })

	// Fill the derived fields once, now, from whatever the name starts as.
	w.onNameChanged()

	// Anything passed on the command line wins over the guess.
	if d := w.opts.Defaults; d.AppID != "" {
		w.appID.SetText(d.AppID)
		w.derived.appID = false
	}
	if d := w.opts.Defaults; d.Module != "" {
		w.module.SetText(d.Module)
		w.derived.module = false
	}
	if d := w.opts.Defaults; d.Binary != "" {
		w.binary.SetText(d.Binary)
		w.derived.binary = false
	}

	w.banner = adw.NewBanner("")
	w.banner.SetRevealed(false)

	w.create = gtk.NewButtonWithLabel("Create Project")
	w.create.AddCSSClass("suggested-action")
	w.create.ConnectClicked(w.onCreate)

	header := adw.NewHeaderBar()
	header.SetTitleWidget(adw.NewWindowTitle("Joinery", "Go · GTK4 · libadwaita"))
	header.PackEnd(w.create)

	scroll := gtk.NewScrolledWindow()
	scroll.SetVExpand(true)
	scroll.SetChild(page)

	body := gtk.NewBox(gtk.OrientationVertical, 0)
	body.Append(w.banner)
	body.Append(scroll)

	tv := adw.NewToolbarView()
	tv.AddTopBar(header)
	tv.SetContent(body)
	return tv
}

// onNameChanged refreshes the fields the user has not taken over.
func (w *wizard) onNameChanged() {
	name := w.name.Text()

	if w.derived.appID {
		w.setDerived(w.appID, scaffold.SuggestAppID(name), &w.derived.appID)
	}
	if w.derived.binary {
		w.setDerived(w.binary, scaffold.SuggestBinary(name), &w.derived.binary)
	}
	if w.derived.module {
		w.setDerived(w.module, scaffold.SuggestModule(name), &w.derived.module)
	}
}

// setDerived writes a guessed value without the write being mistaken for the
// user editing the field.
//
// SetText fires notify::text, and the handler on that field is the one that
// records "the user has taken this over". Without the flag dance below, the
// wizard's own guess would immediately stop it from guessing again.
func (w *wizard) setDerived(row *adw.EntryRow, value string, flag *bool) {
	row.SetText(value)
	*flag = true
}

func (w *wizard) chooseDestination() {
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Where should the project go?")

	if parent := filepath.Dir(w.destination); parent != "" {
		dialog.SetInitialFolder(gio.NewFileForPath(parent))
	}

	dialog.SelectFolder(context.Background(), &w.win.Window,
		func(result gio.AsyncResulter) {
			folder, err := dialog.SelectFolderFinish(result)
			if err != nil {
				// The user pressed Cancel, which arrives here as an error. It
				// is not one.
				return
			}
			w.destination = filepath.Join(folder.Path(), scaffold.SuggestBinary(w.name.Text()))
			w.dir.SetSubtitle(w.destination)
		},
	)
}

func (w *wizard) project() scaffold.Project {
	return scaffold.Project{
		Name:          strings.TrimSpace(w.name.Text()),
		AppID:         strings.TrimSpace(w.appID.Text()),
		Module:        strings.TrimSpace(w.module.Text()),
		Binary:        strings.TrimSpace(w.binary.Text()),
		Author:        strings.TrimSpace(w.author.Text()),
		Dir:           w.destination,
		WithNotes:     w.notes.Active(),
		WithDashboard: w.dashboard.Active() && w.notes.Active(),
	}
}

func (w *wizard) onCreate() {
	p := w.project()

	if err := p.Validate(); err != nil {
		w.showError(err.Error())
		return
	}

	w.banner.SetRevealed(false)
	w.create.SetSensitive(false)

	progress := newProgressPage(p.Dir)
	w.nav.Push(adw.NewNavigationPage(progress.widget(), "Creating"))

	// Generation touches the filesystem and may shell out to git and go, none
	// of which belong on the thread driving the UI. Everything below runs on a
	// goroutine and comes back through glib.IdleAdd.
	go func() {
		err := w.generate(p, progress)

		glib.IdleAdd(func() {
			w.create.SetSensitive(true)
			if err != nil {
				w.nav.Pop()
				w.showError(err.Error())
				return
			}
			w.nav.Push(adw.NewNavigationPage(w.donePage(p), "Done"))
		})
	}()
}

// generate does the work, reporting each step back to the progress page.
func (w *wizard) generate(p scaffold.Project, progress *progressPage) error {
	progress.step("Writing project files…")

	result, err := scaffold.Generate(p, w.opts.TemplateDir)
	if err != nil {
		return err
	}
	progress.step(fmt.Sprintf("Wrote %d files.", result.Files))

	if w.gitInit.Active() {
		progress.step("Initialising a git repository…")
		if err := run(p.Dir, "git", "init", "--quiet"); err != nil {
			// A missing git is not a reason to throw away a project that has
			// already been written. Say so and carry on.
			progress.step("Could not run git init: " + err.Error())
		}
	}

	if w.tidy.Active() {
		progress.step("Downloading dependencies (go mod tidy)…")
		if err := run(p.Dir, "go", "mod", "tidy"); err != nil {
			return fmt.Errorf("go mod tidy failed: %w", err)
		}
		progress.step("Dependencies are ready.")
	}

	return nil
}

func (w *wizard) donePage(p scaffold.Project) gtk.Widgetter {
	status := adw.NewStatusPage()
	status.SetIconName("emblem-ok-symbolic")
	status.SetTitle(p.Name + " is ready")
	status.SetDescription(p.Dir)

	next := gtk.NewLabel("")
	next.SetUseMarkup(true)
	next.SetSelectable(true)
	next.SetMarkup(fmt.Sprintf(
		"<tt>cd %s\nmake run</tt>",
		glib.MarkupEscapeText(p.Dir),
	))
	next.AddCSSClass("card")
	next.SetMarginTop(12)
	next.SetMarginBottom(12)
	next.SetMarginStart(12)
	next.SetMarginEnd(12)

	open := gtk.NewButtonWithLabel("Open Folder")
	open.AddCSSClass("suggested-action")
	open.AddCSSClass("pill")
	open.ConnectClicked(func() {
		// xdg-open detaches, so a failure here is not worth interrupting the
		// user over: they already have the path on screen.
		_ = exec.Command("xdg-open", p.Dir).Start()
	})

	quit := gtk.NewButtonWithLabel("Close")
	quit.AddCSSClass("pill")
	quit.ConnectClicked(func() { w.app.Quit() })

	buttons := gtk.NewBox(gtk.OrientationHorizontal, 12)
	buttons.SetHAlign(gtk.AlignCenter)
	buttons.Append(open)
	buttons.Append(quit)

	box := gtk.NewBox(gtk.OrientationVertical, 12)
	box.Append(next)
	box.Append(buttons)
	status.SetChild(box)

	tv := adw.NewToolbarView()
	tv.AddTopBar(adw.NewHeaderBar())
	tv.SetContent(status)
	return tv
}

func (w *wizard) showError(msg string) {
	w.banner.SetTitle(msg)
	w.banner.SetRevealed(true)
}

// progressPage is the "working on it" screen.
type progressPage struct {
	status *adw.StatusPage
	log    *gtk.Label
	lines  []string
}

func newProgressPage(dir string) *progressPage {
	p := &progressPage{}

	p.status = adw.NewStatusPage()
	p.status.SetTitle("Creating your project")
	p.status.SetDescription(dir)

	spinner := adw.NewSpinner()
	spinner.SetSizeRequest(48, 48)

	p.log = gtk.NewLabel("")
	p.log.AddCSSClass("dim-label")
	p.log.SetWrap(true)

	box := gtk.NewBox(gtk.OrientationVertical, 18)
	box.SetHAlign(gtk.AlignCenter)
	box.Append(spinner)
	box.Append(p.log)
	p.status.SetChild(box)

	return p
}

func (p *progressPage) widget() gtk.Widgetter {
	tv := adw.NewToolbarView()
	tv.AddTopBar(adw.NewHeaderBar())
	tv.SetContent(p.status)
	return tv
}

// step appends a line to the progress log. It is called from the worker
// goroutine, so it hops back to the main thread before touching the label.
func (p *progressPage) step(line string) {
	glib.IdleAdd(func() {
		p.lines = append(p.lines, line)
		if len(p.lines) > 6 {
			p.lines = p.lines[len(p.lines)-6:]
		}
		p.log.SetText(strings.Join(p.lines, "\n"))
	})
}

func run(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func orDefault(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}

func defaultDestination() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return "myapp"
	}
	return filepath.Join(home, "Projects", "myapp")
}

// guessAuthor reads the name git already knows, so the field is usually right
// without anyone typing anything.
func guessAuthor() string {
	out, err := exec.Command("git", "config", "--get", "user.name").Output()
	if err != nil {
		return "Your Name"
	}
	if name := strings.TrimSpace(string(out)); name != "" {
		return name
	}
	return "Your Name"
}
