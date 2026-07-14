// Package eventbus is the in-process, typed pub/sub the modules use to talk to
// each other without knowing each other.
//
// It exists so a module can announce something happened without holding a
// reference to whoever cares. When notes deletes a note, it publishes
// NoteDeleted; whether that redraws a sidebar badge, writes an audit row, or
// does nothing at all is not the notes module's problem.
//
// Reach for an event when the publisher does not need an answer. When it does
// need one ("give me the current theme"), use a service interface from the
// module's api package instead. Events are announcements, not requests.
package eventbus

import (
	"context"
	"log/slog"
	"reflect"
	"sync"
)

// Bus fans events out to the handlers subscribed to their type.
//
// Delivery is synchronous: Publish returns once every handler has run. That
// keeps causality easy to follow in a desktop app, where the alternative
// (events landing at some unspecified later point) turns an ordering bug into a
// heisenbug. Handlers must therefore stay quick; anything slow belongs on a
// goroutine the handler spawns itself.
type Bus struct {
	log *slog.Logger

	mu       sync.RWMutex
	handlers map[reflect.Type][]any
}

// New returns an empty bus. log records handler panics.
func New(log *slog.Logger) *Bus {
	return &Bus{
		log:      log,
		handlers: make(map[reflect.Type][]any),
	}
}

// Subscribe registers fn to receive every event of type E.
//
// Subscriptions are permanent: there is no Unsubscribe, because modules live as
// long as the process does and an unsubscribe API would only invite use-after-
// free bugs in handlers that close over widgets. A handler that should stop
// doing work needs to check its own state.
//
// fn runs on the publisher's goroutine, which is rarely the GTK main thread.
// Anything touching a widget has to go through ui.OnMain.
func Subscribe[E any](b *Bus, fn func(context.Context, E)) {
	t := reflect.TypeFor[E]()

	b.mu.Lock()
	defer b.mu.Unlock()

	b.handlers[t] = append(b.handlers[t], fn)
}

// Publish delivers e to every handler subscribed to E, in subscription order.
//
// A handler that panics is logged and skipped: one misbehaving listener must not
// take down the caller that merely announced a fact, nor stop the listeners
// queued behind it.
func Publish[E any](b *Bus, ctx context.Context, e E) {
	t := reflect.TypeFor[E]()

	b.mu.RLock()
	hs := b.handlers[t]
	// Copy under the read lock: a handler is free to subscribe to something
	// else while running, and we must not iterate a slice being appended to.
	hs = append([]any(nil), hs...)
	b.mu.RUnlock()

	for _, h := range hs {
		deliver(b.log, ctx, h.(func(context.Context, E)), e, t)
	}
}

// deliver is a package-level function rather than a method because Go does not
// allow methods to declare their own type parameters.
func deliver[E any](log *slog.Logger, ctx context.Context, fn func(context.Context, E), e E, t reflect.Type) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("event handler panicked",
				"event", t.String(),
				"panic", r,
			)
		}
	}()
	fn(ctx, e)
}
