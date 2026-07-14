// Command app is the application entry point and its composition root.
//
// This file is the only place that knows every module by name. Everything else
// works through the kernel and the modules' api packages, which is what lets you
// add a feature by writing a directory and one line here, then remove it the
// same way.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"

	"github.com/example/gtkapp/internal/app"
	"github.com/example/gtkapp/internal/app/shell"
	"github.com/example/gtkapp/internal/modules/dashboard"
	"github.com/example/gtkapp/internal/modules/notes"
	"github.com/example/gtkapp/internal/modules/settings"
	"github.com/example/gtkapp/internal/platform/config"
	"github.com/example/gtkapp/internal/platform/database"
	"github.com/example/gtkapp/internal/platform/eventbus"
	"github.com/example/gtkapp/internal/platform/logging"
	"github.com/example/gtkapp/internal/platform/ui"
)

// shutdownTimeout bounds how long modules get to stop cleanly before the process
// exits regardless. A desktop app that hangs on quit is a desktop app the user
// force-kills, and then the next boot has to deal with whatever was half-written.
const shutdownTimeout = 5 * time.Second

func main() {
	log := logging.FromEnv()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	paths, err := config.ResolvePaths(app.ID)
	if err != nil {
		log.Error("resolve application directories", "error", err)
		os.Exit(1)
	}
	if err := paths.EnsureDirs(); err != nil {
		log.Error("create application directories", "error", err)
		os.Exit(1)
	}

	cfg, err := config.Load(paths)
	if err != nil {
		log.Error("load configuration", "error", err)
		os.Exit(1)
	}

	db, err := database.Open(ctx, paths.DatabaseFile())
	if err != nil {
		log.Error("open database", "error", err, "path", paths.DatabaseFile())
		os.Exit(1)
	}
	defer db.Close()

	bus := eventbus.New(log)
	kernel := app.NewKernel(log, cfg, db, bus)

	// The module list. Order decides two things and no more: the order
	// migrations run in, and the reverse order modules stop in. Which module
	// can see which service does not depend on it. That is what the kernel's
	// two-phase boot is for.
	kernel.Mount(
		settings.New(),
		notes.New(),
		dashboard.New(),
	)

	log.Info("starting",
		"app", app.Name,
		"version", app.Version,
		"commit", app.Commit,
		"database", db.Path(),
	)

	application := adw.NewApplication(app.ID, gio.ApplicationFlagsNone)

	// activate fires on launch, and again whenever the user tries to start a
	// second copy. GTK routes that to the running process instead of opening a
	// second window, so this has to be idempotent.
	var window *shell.Shell
	application.ConnectActivate(func() {
		if window != nil {
			window.Window().Present()
			return
		}

		ui.LoadStyles()

		sh := shell.New(application, kernel)

		// Publish the toast surface before booting, so a module's Start can
		// reach for ui.Toaster and find it.
		app.Provide[ui.Toaster](kernel, sh.Toaster())

		if err := kernel.Boot(ctx); err != nil {
			log.Error("boot failed", "error", err)
			application.Quit()
			os.Exit(1)
		}

		sh.Populate(kernel.Views())
		sh.Window().Present()
		window = sh
	})

	// Turn a signal into a real quit.
	//
	// signal.NotifyContext on its own only cancels the context; the GTK main
	// loop knows nothing about it and keeps running, so Ctrl-C in a terminal
	// would leave a window on screen that only a mouse could close. Quit has to
	// be called, and like every GTK call it has to happen on the main thread,
	// which is what ui.OnMain is for.
	go func() {
		<-ctx.Done()
		ui.OnMain(func() { application.Quit() })
	}()

	code := application.Run(os.Args)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := kernel.Shutdown(shutdownCtx); err != nil {
		log.Error("shutdown", "error", err)
	}

	log.Info("stopped")
	os.Exit(code)
}
