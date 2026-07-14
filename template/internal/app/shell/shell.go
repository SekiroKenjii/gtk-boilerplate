// Package shell builds the window every module's screens live inside: the
// sidebar, the content stack, the header bar and the toast overlay.
//
// The shell knows about app.View. It does not know about any module. Modules
// hand it views; it lays them out.
package shell

import (
	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"github.com/example/gtkapp/internal/app"
	"github.com/example/gtkapp/internal/platform/ui"
)

// Shell is the main window and the navigation around it.
type Shell struct {
	kernel *app.Kernel

	window  *adw.ApplicationWindow
	split   *adw.NavigationSplitView
	stack   *gtk.Stack
	sidebar *gtk.ListBox
	title   *adw.WindowTitle
	overlay *adw.ToastOverlay
	toaster ui.Toaster

	views []app.View
	// built records which views have had their Build called. Views are
	// constructed on first visit, so a module the user never opens costs
	// nothing but its registration.
	built map[string]bool
	rows  []app.View // parallel to sidebar rows, indexed by row position
}

// New assembles the window chrome. It deliberately does not add any views yet:
// the caller provides the toaster to the kernel first, then boots the modules,
// then calls Populate. That order is what lets a module's Start reach for
// ui.Toaster and find it.
func New(application *adw.Application, k *app.Kernel) *Shell {
	s := &Shell{
		kernel: k,
		built:  make(map[string]bool),
	}

	s.window = adw.NewApplicationWindow(&application.Application)
	s.window.SetTitle(app.Name)
	s.window.SetDefaultSize(1000, 680)

	s.stack = gtk.NewStack()
	s.stack.SetTransitionType(gtk.StackTransitionTypeCrossfade)

	s.sidebar = gtk.NewListBox()
	s.sidebar.SetSelectionMode(gtk.SelectionSingle)
	s.sidebar.AddCSSClass("navigation-sidebar")
	s.sidebar.ConnectRowSelected(func(row *gtk.ListBoxRow) {
		if row == nil {
			return
		}
		i := row.Index()
		if i < 0 || i >= len(s.rows) {
			return
		}
		s.show(s.rows[i])
	})

	s.split = adw.NewNavigationSplitView()
	s.split.SetSidebar(s.sidebarPage())
	s.split.SetContent(s.contentPage())

	s.overlay = adw.NewToastOverlay()
	s.overlay.SetChild(s.split)
	s.toaster = ui.NewToaster(s.overlay)

	s.window.SetContent(s.overlay)

	// Below 700px the sidebar and the content cannot both fit, so fold them
	// into a single navigable stack. This is the whole of the app's mobile
	// story, and libadwaita does the rest.
	bp := adw.NewBreakpoint(adw.BreakpointConditionParse("max-width: 700px"))
	bp.AddSetter(s.split, "collapsed", true)
	s.window.AddBreakpoint(bp)

	s.installActions(application)

	return s
}

// Toaster is the shell's toast surface. Publish it to the kernel before booting
// so modules can reach it.
func (s *Shell) Toaster() ui.Toaster { return s.toaster }

// Window is the top-level window, for callers that need it as a dialog parent.
func (s *Shell) Window() *adw.ApplicationWindow { return s.window }

// Populate fills the sidebar and content stack from the modules' views, then
// selects the first one. Call it after the kernel has booted.
func (s *Shell) Populate(views []app.View) {
	s.views = views

	for _, v := range views {
		row := adw.NewActionRow()
		row.SetTitle(v.Title)
		if v.Icon != "" {
			row.SetIconName(v.Icon)
		}
		s.sidebar.Append(row)
		s.rows = append(s.rows, v)
	}

	if len(views) == 0 {
		// A build with every module disabled is a legitimate thing to do, and
		// it should look intentional rather than broken.
		empty := adw.NewStatusPage()
		empty.SetIconName("application-x-executable-symbolic")
		empty.SetTitle("No modules")
		empty.SetDescription("Mount a module in cmd/app/main.go to see it here.")
		s.stack.AddNamed(empty, "__empty__")
		s.stack.SetVisibleChildName("__empty__")
		return
	}

	if row := s.sidebar.RowAtIndex(0); row != nil {
		s.sidebar.SelectRow(row)
	}
}

// show lazily builds a view the first time it is opened, then reveals it.
func (s *Shell) show(v app.View) {
	if !s.built[v.ID] {
		s.kernel.Log.Debug("building view", "view", v.ID)
		widget := v.Build(s.kernel)
		s.stack.AddNamed(widget, v.ID)
		s.built[v.ID] = true
	}

	s.stack.SetVisibleChildName(v.ID)
	s.title.SetTitle(v.Title)

	// When the split view is collapsed the sidebar covers the content, so a tap
	// has to push us over to it. Expanded, this is a no-op.
	s.split.SetShowContent(true)
}

func (s *Shell) sidebarPage() *adw.NavigationPage {
	header := adw.NewHeaderBar()
	header.SetTitleWidget(adw.NewWindowTitle(app.Name, ""))

	scroll := gtk.NewScrolledWindow()
	scroll.SetVExpand(true)
	scroll.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroll.SetChild(s.sidebar)

	tv := adw.NewToolbarView()
	tv.AddTopBar(header)
	tv.SetContent(scroll)

	page := adw.NewNavigationPage(tv, app.Name)
	return page
}

func (s *Shell) contentPage() *adw.NavigationPage {
	s.title = adw.NewWindowTitle("", "")

	menu := gio.NewMenu()
	menu.Append("Keyboard Shortcuts", "app.shortcuts")
	menu.Append("About "+app.Name, "app.about")
	menu.Append("Quit", "app.quit")

	menuButton := gtk.NewMenuButton()
	menuButton.SetIconName("open-menu-symbolic")
	menuButton.SetMenuModel(menu)
	menuButton.SetPrimary(true)

	header := adw.NewHeaderBar()
	header.SetTitleWidget(s.title)
	header.PackEnd(menuButton)

	s.stack.SetVExpand(true)

	tv := adw.NewToolbarView()
	tv.AddTopBar(header)
	tv.SetContent(s.stack)

	return adw.NewNavigationPage(tv, app.Name)
}

func (s *Shell) installActions(application *adw.Application) {
	about := gio.NewSimpleAction("about", nil)
	about.ConnectActivate(func(*glib.Variant) { s.showAbout() })
	application.AddAction(about)

	shortcuts := gio.NewSimpleAction("shortcuts", nil)
	shortcuts.ConnectActivate(func(*glib.Variant) { s.showShortcuts() })
	application.AddAction(shortcuts)

	quit := gio.NewSimpleAction("quit", nil)
	quit.ConnectActivate(func(*glib.Variant) { application.Quit() })
	application.AddAction(quit)

	application.SetAccelsForAction("app.quit", []string{"<Control>q"})
	application.SetAccelsForAction("app.shortcuts", []string{"<Control>question"})
}

func (s *Shell) showAbout() {
	about := adw.NewAboutDialog()
	about.SetApplicationName(app.Name)
	about.SetApplicationIcon(app.ID)
	about.SetVersion(app.Version)
	about.SetDeveloperName("Your Name")
	about.SetLicenseType(gtk.LicenseMITX11)
	about.SetComments("A modular monolith desktop application, written in Go with GTK4 and libadwaita.")
	about.Present(s.window)
}

func (s *Shell) showShortcuts() {
	// A real shortcuts window is a fair bit of XML for very little payoff at
	// this stage; a dialog listing what exists is honest and easy to grow.
	dialog := adw.NewAlertDialog(
		"Keyboard Shortcuts",
		"Ctrl+Q\tQuit\nCtrl+?\tThis dialog",
	)
	dialog.AddResponse("close", "Close")
	dialog.Present(s.window)
}
