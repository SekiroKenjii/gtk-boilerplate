// Package ui holds the GTK helpers every module shares: the main-thread bridge,
// stylesheet loading, and the toast surface.
//
// It is the one place in the platform layer that imports GTK. Modules import it;
// it imports no module.
package ui

import (
	"context"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// OnMain schedules fn to run on the GTK main thread and returns immediately.
//
// This is the single most important rule in the codebase. GTK is not
// thread-safe: every widget call has to happen on the thread running the main
// loop. Go, meanwhile, will happily move a goroutine anywhere, and the moment a
// module does its work on a background goroutine (a database read, an HTTP
// call, a file scan), the result arrives on the wrong thread.
//
// Touching a widget from there does not reliably crash. It corrupts GTK's
// internal state and the app falls over somewhere else entirely, minutes later,
// with a stack trace pointing at innocent code. So:
//
//	go func() {
//	    notes, err := svc.List(ctx)      // fine: no widgets here
//	    ui.OnMain(func() {
//	        listView.SetModel(model)     // fine: back on the main thread
//	    })
//	}()
//
// Calling OnMain from the main thread is safe; fn simply runs on the next idle
// turn of the loop rather than inline.
func OnMain(fn func()) {
	glib.IdleAdd(fn)
}

// Async runs work on a background goroutine and hands its result to done on the
// GTK main thread.
//
// It is the pattern every screen in this codebase uses to call a service without
// freezing the window: the UI stays responsive while work runs, and done is free
// to touch widgets because it is already back where GTK wants it.
//
//	ui.Async(ctx,
//	    func(ctx context.Context) ([]notesapi.Note, error) { return svc.List(ctx) },
//	    func(notes []notesapi.Note, err error) {
//	        if err != nil {
//	            page.ShowError(err)
//	            return
//	        }
//	        page.Render(notes)
//	    },
//	)
//
// If ctx is cancelled before work finishes, done is not called: the screen that
// asked for the result is gone, and calling back into its widgets is how you get
// a use-after-free.
func Async[T any](ctx context.Context, work func(context.Context) (T, error), done func(T, error)) {
	go func() {
		result, err := work(ctx)

		if ctx.Err() != nil {
			return
		}

		OnMain(func() {
			// Re-check on the main thread: cancellation may well have happened
			// while this callback sat in the idle queue.
			if ctx.Err() != nil {
				return
			}
			done(result, err)
		})
	}()
}

// Do is Async for work that returns only an error.
func Do(ctx context.Context, work func(context.Context) error, done func(error)) {
	Async(ctx,
		func(ctx context.Context) (struct{}, error) {
			return struct{}{}, work(ctx)
		},
		func(_ struct{}, err error) {
			done(err)
		},
	)
}
