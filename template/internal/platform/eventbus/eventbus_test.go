package eventbus_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/example/gtkapp/internal/platform/eventbus"
)

type userCreated struct{ Name string }
type userDeleted struct{ Name string }

func newBus() *eventbus.Bus {
	return eventbus.New(slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestPublishReachesEverySubscriber(t *testing.T) {
	bus := newBus()

	var got []string
	eventbus.Subscribe(bus, func(_ context.Context, e userCreated) {
		got = append(got, "first:"+e.Name)
	})
	eventbus.Subscribe(bus, func(_ context.Context, e userCreated) {
		got = append(got, "second:"+e.Name)
	})

	eventbus.Publish(bus, context.Background(), userCreated{Name: "ada"})

	// Delivery is synchronous and ordered, so this is a plain equality check
	// rather than a race-prone wait.
	want := []string{"first:ada", "second:ada"}
	if len(got) != len(want) {
		t.Fatalf("handlers ran %d times, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("call %d = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestEventsAreRoutedByType(t *testing.T) {
	bus := newBus()

	created, deleted := 0, 0
	eventbus.Subscribe(bus, func(context.Context, userCreated) { created++ })
	eventbus.Subscribe(bus, func(context.Context, userDeleted) { deleted++ })

	eventbus.Publish(bus, context.Background(), userCreated{Name: "ada"})

	if created != 1 {
		t.Errorf("userCreated handler ran %d times, want 1", created)
	}
	if deleted != 0 {
		t.Errorf("userDeleted handler ran %d times; it must not see a different event", deleted)
	}
}

func TestPublishWithNoSubscribersIsFine(t *testing.T) {
	bus := newBus()

	// A module publishing into the void is the normal case, not an edge one:
	// whoever cared may simply not be compiled into this build.
	eventbus.Publish(bus, context.Background(), userCreated{Name: "ada"})
}

func TestPanickingHandlerDoesNotStopTheOthers(t *testing.T) {
	bus := newBus()

	reached := false
	eventbus.Subscribe(bus, func(context.Context, userCreated) {
		panic("this handler is broken")
	})
	eventbus.Subscribe(bus, func(context.Context, userCreated) {
		reached = true
	})

	// The point of the recover in Publish: one bad listener must not take down
	// the caller that merely announced a fact, nor the listeners behind it.
	eventbus.Publish(bus, context.Background(), userCreated{Name: "ada"})

	if !reached {
		t.Error("a panicking handler stopped the handler queued behind it")
	}
}

func TestContextReachesTheHandler(t *testing.T) {
	bus := newBus()

	type key struct{}
	ctx := context.WithValue(context.Background(), key{}, "carried")

	var seen any
	eventbus.Subscribe(bus, func(ctx context.Context, _ userCreated) {
		seen = ctx.Value(key{})
	})

	eventbus.Publish(bus, ctx, userCreated{Name: "ada"})

	if seen != "carried" {
		t.Errorf("handler saw context value %v, want %q", seen, "carried")
	}
}
