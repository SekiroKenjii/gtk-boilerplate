// Package service implements the notes use cases. It is private to the module.
package service

import (
	"context"
	"time"

	notesapi "github.com/example/gtkapp/internal/modules/notes/api"
	"github.com/example/gtkapp/internal/modules/notes/domain"
	"github.com/example/gtkapp/internal/modules/notes/store"
	"github.com/example/gtkapp/internal/platform/eventbus"
)

// Clock hands the service the current time.
//
// It exists so tests can pin "now" instead of sleeping or asserting on ranges.
// The production value is time.Now, and that is the only place the real clock is
// read.
type Clock func() time.Time

// Service is the notesapi.Service implementation.
type Service struct {
	store *store.SQLite
	bus   *eventbus.Bus
	now   Clock
}

// New builds the service. Pass nil for clock to use the wall clock.
func New(st *store.SQLite, bus *eventbus.Bus, clock Clock) *Service {
	if clock == nil {
		clock = time.Now
	}
	return &Service{store: st, bus: bus, now: clock}
}

// List returns every note, newest first.
func (s *Service) List(ctx context.Context) ([]notesapi.Note, error) {
	notes, err := s.store.List(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]notesapi.Note, len(notes))
	for i, n := range notes {
		out[i] = toAPI(n)
	}
	return out, nil
}

// Get returns one note.
func (s *Service) Get(ctx context.Context, id int64) (notesapi.Note, error) {
	n, err := s.store.Get(ctx, id)
	if err != nil {
		return notesapi.Note{}, err
	}
	return toAPI(n), nil
}

// Create validates and stores a new note, then announces it.
func (s *Service) Create(ctx context.Context, title, body string) (notesapi.Note, error) {
	// The domain decides whether this is a legal note. The service's job is to
	// orchestrate (ask the domain, then the store), not to re-implement the
	// rules.
	n, err := domain.NewNote(title, body, s.now())
	if err != nil {
		return notesapi.Note{}, err
	}

	stored, err := s.store.Insert(ctx, n)
	if err != nil {
		return notesapi.Note{}, err
	}

	out := toAPI(stored)
	eventbus.Publish(s.bus, ctx, notesapi.Created{Note: out})
	return out, nil
}

// Update rewrites a note.
func (s *Service) Update(ctx context.Context, id int64, title, body string) (notesapi.Note, error) {
	// Read before write: the entity has to exist before its invariants can be
	// re-checked, and CreatedAt has to survive the round trip.
	n, err := s.store.Get(ctx, id)
	if err != nil {
		return notesapi.Note{}, err
	}

	now := s.now()
	if err := n.Rename(title, now); err != nil {
		return notesapi.Note{}, err
	}
	n.Rewrite(body, now)

	if err := s.store.Update(ctx, n); err != nil {
		return notesapi.Note{}, err
	}

	out := toAPI(n)
	eventbus.Publish(s.bus, ctx, notesapi.Updated{Note: out})
	return out, nil
}

// Delete removes a note and announces it.
func (s *Service) Delete(ctx context.Context, id int64) error {
	// Fetch first, purely so the event can carry the title. Subscribers cannot
	// look it up afterwards. That is the whole problem with deletion events.
	n, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}

	if err := s.store.Delete(ctx, id); err != nil {
		return err
	}

	eventbus.Publish(s.bus, ctx, notesapi.Deleted{ID: id, Title: n.Title})
	return nil
}

// Count returns how many notes exist.
func (s *Service) Count(ctx context.Context) (int, error) {
	return s.store.Count(ctx)
}

// toAPI maps the domain entity onto the DTO other modules see. It is the border
// checkpoint: nothing crosses out of the module without passing through here.
func toAPI(n domain.Note) notesapi.Note {
	return notesapi.Note{
		ID:        n.ID,
		Title:     n.Title,
		Body:      n.Body,
		CreatedAt: n.CreatedAt,
		UpdatedAt: n.UpdatedAt,
	}
}

var _ notesapi.Service = (*Service)(nil)
