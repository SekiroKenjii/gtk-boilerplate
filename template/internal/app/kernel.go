package app

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"sync"

	"github.com/example/gtkapp/internal/platform/config"
	"github.com/example/gtkapp/internal/platform/database"
	"github.com/example/gtkapp/internal/platform/eventbus"
)

// Kernel is the composition root. It owns the platform services every module is
// allowed to reach for directly, plus the registry through which modules reach
// each other.
//
// Modules receive the kernel; the kernel never imports a module. That one-way
// rule is what keeps the dependency graph acyclic, and tools/archlint fails the
// build if it is broken.
type Kernel struct {
	Log *slog.Logger
	Cfg *config.Config
	DB  *database.DB
	Bus *eventbus.Bus

	mu       sync.RWMutex
	services map[reflect.Type]any

	modules []Module
	started []Stopper
}

// NewKernel wires the platform services together. Modules are added afterwards
// with Mount.
func NewKernel(log *slog.Logger, cfg *config.Config, db *database.DB, bus *eventbus.Bus) *Kernel {
	return &Kernel{
		Log:      log,
		Cfg:      cfg,
		DB:       db,
		Bus:      bus,
		services: make(map[reflect.Type]any),
	}
}

// Mount adds modules in the order they should register. Cross-module service
// lookups do not depend on this order. Only migrations and shutdown do.
func (k *Kernel) Mount(mods ...Module) {
	k.modules = append(k.modules, mods...)
}

// Modules returns the mounted modules in registration order.
func (k *Kernel) Modules() []Module {
	return k.modules
}

// Boot runs the module lifecycle: Register for everyone, then Migrate for
// everyone, then Start for everyone. Splitting it this way is what makes the
// registration order irrelevant to service resolution.
//
// If any stage fails, the modules already started are stopped before the error
// is returned, so a failed boot does not leak goroutines or file handles.
func (k *Kernel) Boot(ctx context.Context) error {
	for _, m := range k.modules {
		if err := m.Register(k); err != nil {
			return fmt.Errorf("register module %q: %w", m.ID(), err)
		}
		k.Log.Debug("module registered", "module", m.ID())
	}

	for _, m := range k.modules {
		mig, ok := m.(Migrator)
		if !ok {
			continue
		}
		if err := k.DB.Migrate(ctx, m.ID(), toDBMigrations(mig.Migrations())); err != nil {
			return fmt.Errorf("migrate module %q: %w", m.ID(), err)
		}
	}

	for _, m := range k.modules {
		if s, ok := m.(Starter); ok {
			if err := s.Start(ctx, k); err != nil {
				k.Shutdown(ctx)
				return fmt.Errorf("start module %q: %w", m.ID(), err)
			}
		}
		if s, ok := m.(Stopper); ok {
			k.started = append(k.started, s)
		}
		k.Log.Debug("module started", "module", m.ID())
	}

	return nil
}

// Shutdown stops every started module in reverse order. It keeps going after a
// failure and reports the first error it saw: one module refusing to stop must
// not strand the rest.
func (k *Kernel) Shutdown(ctx context.Context) error {
	var first error
	for i := len(k.started) - 1; i >= 0; i-- {
		if err := k.started[i].Stop(ctx); err != nil && first == nil {
			first = err
		}
	}
	k.started = nil
	return first
}

// Views collects the screens contributed by every module, sorted for the
// sidebar.
func (k *Kernel) Views() []View {
	var views []View
	for _, m := range k.modules {
		c, ok := m.(UIContributor)
		if !ok {
			continue
		}
		views = append(views, c.Views()...)
	}
	sort.SliceStable(views, func(i, j int) bool {
		if views[i].Order != views[j].Order {
			return views[i].Order < views[j].Order
		}
		return views[i].Title < views[j].Title
	})
	return views
}

func toDBMigrations(in []Migration) []database.Migration {
	out := make([]database.Migration, len(in))
	for i, m := range in {
		out[i] = database.Migration{Name: m.Name, SQL: m.SQL}
	}
	return out
}

// Provide publishes a module's service under the interface type T. Call it from
// Register.
//
// T is meant to be an interface declared in the providing module's api package.
// Publishing a concrete struct works but defeats the point: consumers would
// then compile against your internals.
//
//	app.Provide[notesapi.Service](k, svc)
//
// Providing the same type twice is a programming error and panics. Two modules
// claiming the same contract is a design mistake worth failing loudly on, at
// startup, rather than silently letting the last one win.
func Provide[T any](k *Kernel, impl T) {
	t := reflect.TypeFor[T]()

	k.mu.Lock()
	defer k.mu.Unlock()

	if _, dup := k.services[t]; dup {
		panic(fmt.Sprintf("app: service %s provided twice", t))
	}
	k.services[t] = impl
}

// Use resolves the service published under T. Call it from Start or from inside
// a view's Build, never from Register: at Register time the module that
// provides T may not have run yet.
//
// It panics when T is missing, because a module asking for a contract nobody
// implements cannot do anything useful: better a stack trace at boot than a nil
// dereference three screens in. Use TryUse when the dependency is genuinely
// optional.
func Use[T any](k *Kernel) T {
	v, ok := TryUse[T](k)
	if !ok {
		panic(fmt.Sprintf("app: no service provides %s", reflect.TypeFor[T]()))
	}
	return v
}

// TryUse resolves T, reporting whether it was found. Use it for optional
// dependencies, e.g. a feature that lights up only when some module is
// compiled in.
func TryUse[T any](k *Kernel) (T, bool) {
	t := reflect.TypeFor[T]()

	k.mu.RLock()
	defer k.mu.RUnlock()

	v, ok := k.services[t]
	if !ok {
		var zero T
		return zero, false
	}
	return v.(T), true
}

// Subscribe registers fn as a listener for events of type E published by any
// module. It is a thin, kernel-flavoured wrapper over the bus, kept here so a
// module never has to reach past the kernel.
//
// fn runs on the goroutine that publishes, which is usually not the GTK main
// thread. Wrap any widget work in ui.OnMain.
func Subscribe[E any](k *Kernel, fn func(context.Context, E)) {
	eventbus.Subscribe(k.Bus, fn)
}

// Publish delivers e to every subscriber of type E.
func Publish[E any](k *Kernel, ctx context.Context, e E) {
	eventbus.Publish(k.Bus, ctx, e)
}
