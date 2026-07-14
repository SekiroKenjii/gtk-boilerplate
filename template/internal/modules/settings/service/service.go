// Package service implements the settings module's use cases.
//
// It is internal to the module. Nothing outside internal/modules/settings/ may
// import it. Callers go through settingsapi.Store.
package service

import (
	"context"
	"sync"

	settingsapi "github.com/example/gtkapp/internal/modules/settings/api"
	"github.com/example/gtkapp/internal/platform/config"
	"github.com/example/gtkapp/internal/platform/errs"
	"github.com/example/gtkapp/internal/platform/eventbus"
)

// configKey namespaces this module's slice of the config file. It matches the
// module ID, which is the convention the whole codebase follows.
const configKey = "settings"

// Store is the settingsapi.Store implementation.
//
// The current settings are cached in memory and written through to disk on every
// change. Reads therefore never touch the filesystem, which matters because the
// UI calls Get on paths where a stall would be visible.
type Store struct {
	cfg *config.Config
	bus *eventbus.Bus

	mu      sync.RWMutex
	current settingsapi.Settings
}

// New loads the persisted settings, falling back to defaults.
//
// A config file holding a theme this build does not recognise is repaired
// rather than rejected: refusing to start because of one bad string in a
// preferences file would be a poor trade.
func New(cfg *config.Config, bus *eventbus.Bus) (*Store, error) {
	s := settingsapi.Defaults()

	if _, err := config.Get(cfg, configKey, &s); err != nil {
		return nil, errs.Internalf(err, "load settings")
	}
	if !s.Theme.Valid() {
		s.Theme = settingsapi.Defaults().Theme
	}

	return &Store{cfg: cfg, bus: bus, current: s}, nil
}

// Get returns a copy of the current settings.
func (s *Store) Get() settingsapi.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

// Update validates s, persists it, and announces the change.
//
// The event fires only after the write succeeds. Subscribers can therefore treat
// Changed as a fact rather than an intention, and nobody ends up rendering a
// theme that failed to save.
func (s *Store) Update(ctx context.Context, next settingsapi.Settings) error {
	if !next.Theme.Valid() {
		return errs.Invalidf("%q is not a theme this application knows", next.Theme)
	}

	if err := config.Set(s.cfg, configKey, next); err != nil {
		return errs.Internalf(err, "save settings")
	}

	s.mu.Lock()
	s.current = next
	s.mu.Unlock()

	eventbus.Publish(s.bus, ctx, settingsapi.Changed{Settings: next})
	return nil
}
