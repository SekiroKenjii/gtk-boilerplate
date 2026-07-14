// Package config loads and stores the application's settings.
//
// Files land in the XDG directories, so the app behaves the way a Linux desktop
// user expects: config in ~/.config/<app-id>/, data in
// ~/.local/share/<app-id>/, and both overridable by the usual environment
// variables. Under Flatpak the runtime rewrites those variables for us and
// everything lands inside the sandbox with no code change.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

// Config is the process-wide settings document.
//
// Values live in a single JSON file rather than GSettings. GSettings would be
// the GNOME-native answer, but it needs a compiled schema installed system-wide
// before the app will even start, which breaks `go run .` on a fresh checkout.
// That is too high a price for a boilerplate whose whole promise is that it
// runs on the first command.
type Config struct {
	// Values holds module settings, namespaced by module ID. The settings
	// module owns the typed accessors; everyone else goes through its api.
	Values map[string]json.RawMessage `json:"values"`

	mu   sync.RWMutex
	path string
}

// Paths are the resolved XDG locations for one application ID.
type Paths struct {
	Config string // ~/.config/<app-id>
	Data   string // ~/.local/share/<app-id>
	State  string // ~/.local/state/<app-id>
	Cache  string // ~/.cache/<app-id>
}

// ResolvePaths computes the XDG directories for appID without creating them.
func ResolvePaths(appID string) (Paths, error) {
	cfgHome, err := os.UserConfigDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve config dir: %w", err)
	}
	cacheHome, err := os.UserCacheDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve cache dir: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, fmt.Errorf("resolve home dir: %w", err)
	}

	// Go has no os.UserDataDir or os.UserStateDir, so spell out the XDG rules
	// the same way the spec does.
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local", "share")
	}
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		stateHome = filepath.Join(home, ".local", "state")
	}

	return Paths{
		Config: filepath.Join(cfgHome, appID),
		Data:   filepath.Join(dataHome, appID),
		State:  filepath.Join(stateHome, appID),
		Cache:  filepath.Join(cacheHome, appID),
	}, nil
}

// EnsureDirs creates every directory in p.
func (p Paths) EnsureDirs() error {
	for _, dir := range []string{p.Config, p.Data, p.State, p.Cache} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// DatabaseFile is where the SQLite store lives.
func (p Paths) DatabaseFile() string {
	return filepath.Join(p.Data, "app.db")
}

// Load reads config.json from p.Config. A missing file is not an error: it is
// simply the first run, and an empty config is the correct starting point.
func Load(p Paths) (*Config, error) {
	path := filepath.Join(p.Config, "config.json")

	c := &Config{
		Values: make(map[string]json.RawMessage),
		path:   path,
	}

	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if c.Values == nil {
		c.Values = make(map[string]json.RawMessage)
	}
	return c, nil
}

// Get decodes the settings blob stored under key into v. It reports false when
// nothing is stored yet, leaving v untouched so the caller's defaults survive.
func Get[T any](c *Config, key string, v *T) (bool, error) {
	c.mu.RLock()
	raw, ok := c.Values[key]
	c.mu.RUnlock()

	if !ok {
		return false, nil
	}
	if err := json.Unmarshal(raw, v); err != nil {
		return false, fmt.Errorf("decode config key %q: %w", key, err)
	}
	return true, nil
}

// Set stores v under key and writes the file.
//
// It saves on every call rather than batching. A desktop app's settings change
// when a human flips a switch, so the write rate is measured in "a few per
// minute". The alternative, losing a preference because the app was killed
// before some flush timer fired, is exactly the kind of small betrayal users
// remember.
func Set[T any](c *Config, key string, v T) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode config key %q: %w", key, err)
	}

	c.mu.Lock()
	c.Values[key] = raw
	c.mu.Unlock()

	return c.save()
}

func (c *Config) save() error {
	c.mu.RLock()
	data, err := json.MarshalIndent(c, "", "  ")
	c.mu.RUnlock()
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	// Write to a sibling temp file and rename over the target. Rename is atomic
	// within a filesystem, so a crash mid-write leaves the previous config
	// intact instead of a half-written file the next boot refuses to parse.
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	if err := os.Rename(tmp, c.path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("replace config: %w", err)
	}
	return nil
}
