package service_test

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	notesapi "github.com/example/gtkapp/internal/modules/notes/api"
	"github.com/example/gtkapp/internal/modules/notes/service"
	"github.com/example/gtkapp/internal/modules/notes/store"
	"github.com/example/gtkapp/internal/platform/database"
	"github.com/example/gtkapp/internal/platform/errs"
	"github.com/example/gtkapp/internal/platform/eventbus"
)

// The service talks to a real SQLite database (in memory, created fresh per
// test). Mocking the store would test the mock; this tests the SQL, the
// migrations and the service together, and still runs in milliseconds.
//
// What it does not need is a display. No GTK is imported anywhere below, which
// is the point of keeping the UI out of the service layer: this suite runs on a
// headless CI box with no X server and no Wayland socket.

var testTime = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func newService(t *testing.T) (*service.Service, *eventbus.Bus, context.Context) {
	t.Helper()

	ctx := context.Background()

	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.Migrate(ctx, "notes", store.Migrations()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	bus := eventbus.New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// A fixed clock, so assertions on timestamps are exact rather than "within
	// a second of now".
	clock := func() time.Time { return testTime }

	return service.New(store.New(db), bus, clock), bus, ctx
}

func TestCreateThenList(t *testing.T) {
	svc, _, ctx := newService(t)

	created, err := svc.Create(ctx, "Shopping list", "milk")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == 0 {
		t.Error("Create returned a note with no ID")
	}
	if !created.CreatedAt.Equal(testTime) {
		t.Errorf("CreatedAt = %v, want %v", created.CreatedAt, testTime)
	}

	notes, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(notes) != 1 {
		t.Fatalf("List returned %d notes, want 1", len(notes))
	}
	if notes[0].Title != "Shopping list" {
		t.Errorf("title = %q, want %q", notes[0].Title, "Shopping list")
	}
}

func TestCreateRejectsBlankTitle(t *testing.T) {
	svc, _, ctx := newService(t)

	_, err := svc.Create(ctx, "   ", "body")
	if err == nil {
		t.Fatal("expected an error for a blank title")
	}
	if got := errs.KindOf(err); got != errs.Invalid {
		t.Errorf("kind = %v, want Invalid", got)
	}

	// A rejected create must not leave a row behind.
	n, err := svc.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Errorf("Count = %d after a rejected create, want 0", n)
	}
}

func TestCreatePublishesCreated(t *testing.T) {
	svc, bus, ctx := newService(t)

	var got []notesapi.Created
	eventbus.Subscribe(bus, func(_ context.Context, e notesapi.Created) {
		got = append(got, e)
	})

	if _, err := svc.Create(ctx, "Shopping list", ""); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("Created fired %d times, want 1", len(got))
	}
	if got[0].Note.Title != "Shopping list" {
		t.Errorf("event carried title %q, want %q", got[0].Note.Title, "Shopping list")
	}
	if got[0].Note.ID == 0 {
		t.Error("event carried a note with no ID; subscribers cannot act on that")
	}
}

func TestFailedCreatePublishesNothing(t *testing.T) {
	svc, bus, ctx := newService(t)

	fired := 0
	eventbus.Subscribe(bus, func(context.Context, notesapi.Created) { fired++ })

	if _, err := svc.Create(ctx, "", "body"); err == nil {
		t.Fatal("expected an error")
	}

	// Events are facts. A create that failed is not one.
	if fired != 0 {
		t.Errorf("Created fired %d times after a failed create, want 0", fired)
	}
}

func TestGetMissingNoteIsNotFound(t *testing.T) {
	svc, _, ctx := newService(t)

	_, err := svc.Get(ctx, 404)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := errs.KindOf(err); got != errs.NotFound {
		t.Errorf("kind = %v, want NotFound", got)
	}
}

func TestUpdateKeepsCreatedAt(t *testing.T) {
	svc, _, ctx := newService(t)

	created, err := svc.Create(ctx, "Before", "old body")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated, err := svc.Update(ctx, created.ID, "After", "new body")
	if err != nil {
		t.Fatalf("Update: %v", err)
	}

	if updated.Title != "After" || updated.Body != "new body" {
		t.Errorf("Update = %q/%q, want %q/%q", updated.Title, updated.Body, "After", "new body")
	}
	if !updated.CreatedAt.Equal(created.CreatedAt) {
		t.Errorf("CreatedAt = %v after update, want it unchanged at %v",
			updated.CreatedAt, created.CreatedAt)
	}
}

func TestDeletePublishesTheTitle(t *testing.T) {
	svc, bus, ctx := newService(t)

	var got []notesapi.Deleted
	eventbus.Subscribe(bus, func(_ context.Context, e notesapi.Deleted) {
		got = append(got, e)
	})

	created, err := svc.Create(ctx, "Shopping list", "")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("Deleted fired %d times, want 1", len(got))
	}
	// The title has to ride along on the event: by the time a subscriber runs,
	// the row is gone and it could not look the name up if it wanted to.
	if got[0].Title != "Shopping list" {
		t.Errorf("event carried title %q, want %q", got[0].Title, "Shopping list")
	}

	n, err := svc.Count(ctx)
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if n != 0 {
		t.Errorf("Count = %d after delete, want 0", n)
	}
}

func TestDeleteMissingNoteIsNotFound(t *testing.T) {
	svc, _, ctx := newService(t)

	err := svc.Delete(ctx, 404)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := errs.KindOf(err); got != errs.NotFound {
		t.Errorf("kind = %v, want NotFound", got)
	}
}

func TestListIsNewestFirst(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(ctx, ":memory:")
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	if err := db.Migrate(ctx, "notes", store.Migrations()); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	bus := eventbus.New(slog.New(slog.NewTextHandler(io.Discard, nil)))

	// A clock that advances a minute per call, so the three notes get distinct,
	// increasing timestamps and the ordering has something to sort on.
	tick := testTime
	clock := func() time.Time {
		tick = tick.Add(time.Minute)
		return tick
	}
	svc := service.New(store.New(db), bus, clock)

	for _, title := range []string{"first", "second", "third"} {
		if _, err := svc.Create(ctx, title, ""); err != nil {
			t.Fatalf("Create(%q): %v", title, err)
		}
	}

	notes, err := svc.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	want := []string{"third", "second", "first"}
	if len(notes) != len(want) {
		t.Fatalf("List returned %d notes, want %d", len(notes), len(want))
	}
	for i, title := range want {
		if notes[i].Title != title {
			t.Errorf("notes[%d] = %q, want %q", i, notes[i].Title, title)
		}
	}
}
