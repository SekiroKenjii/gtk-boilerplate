# Architecture

This is a modular monolith: one process, one binary, one database. Inside
it are modules as separate as if they were services, without any of the network
in between.

## Why not microservices, why not a plain monolith

A desktop app is a single process by definition. There is no fleet to scale and
no team boundary to defend with an API gateway, so the usual arguments for
splitting a system across the network do not apply.

What does apply is the reason people split systems up in the first place: after
a year, nobody can change anything without breaking something else, because
everything reaches into everything.

A modular monolith takes the discipline and leaves the distribution. Modules
publish contracts and hide their internals. They are compiled together and run
together, so a call is a function call rather than an RPC, and a refactor across
two modules is one commit rather than a deployment plan.

The catch is that nothing enforces it. In a microservice, the network stops you
reaching into someone else's database. Here, an import statement is all it takes.
So the boundary has to be checked by a tool, and that tool has to run in CI. That
is what `tools/archlint` is, and it is not optional.

## The layers

```text
cmd/app/main.go        composition root: knows every module, is known by none
    |
    v
internal/app/          the kernel: the Module contract, the registry, the shell
    |
    v
internal/modules/*/    the features
    |
    v
internal/platform/     config, logging, database, event bus, GTK helpers
```

Dependencies point downward, and only downward:

- `cmd/` may import anything. It is the only place that mounts modules.
- A module may import the platform, and other modules' `api` packages.
- The platform may import nothing above it. It does not know modules exist.
- The kernel may not import a module either. It defines the contract they
  implement; it does not know who implements it.

Each of those is a rule in `archlint`, and each of them fails the build.

## Anatomy of a module

```text
internal/modules/notes/
  api/        the contract: interfaces, DTOs, events. The only importable package.
  domain/     entities and the rules they enforce. No SQL, no GTK.
  store/      persistence. Owns every table prefixed notes_.
  service/    use cases. Orchestrates domain and store; publishes events.
  ui/         screens.
  module.go   the wiring.
```

`api/` is the whole of the module's public surface. It holds no logic, imports no
other module, and touches neither GTK nor SQL. Everything else is private, and
free to change, as long as `api/` keeps its promises.

`domain/` is where the rules live, and it is the layer that pays for itself
fastest. Because it imports nothing but the platform's error types, its tests
need no database and no display: they run in milliseconds, anywhere.

## The lifecycle

Every module goes through the same stages, and every module finishes a stage
before any module starts the next:

1. **Register.** Publish your services, subscribe to events. Do not resolve
   anything: the modules after you have not run yet.
2. **Migrate.** Create the tables you own.
3. **Start.** Resolve what you need, open what you need, spawn what you need.
4. **Views.** Hand the shell your screens.
5. **Stop.** Release what Start acquired, in reverse order.

That split is the point. Because *everyone* registers before *anyone* starts,
mount order does not decide who can see whom. A module resolving
`settingsapi.Store` in `Start` finds it whether settings was mounted first or
last, and nobody has to maintain a dependency-ordered list in `main.go`.

Mount order still decides two things, and only two: the order migrations run in,
and the reverse order modules stop in.

## Two ways for modules to talk

**Ask**, when you need an answer:

```go
store := app.Use[settingsapi.Store](k)
if store.Get().ConfirmDelete {
    // ...
}
```

`Use[T]` resolves the interface `T` from the kernel's registry. The type
parameter is the contract, not the implementation. The consumer compiles against
`settingsapi.Store`, so the settings module can rewrite everything behind it
without a single caller noticing.

`Use` panics when nothing provides `T`. That is deliberate. A module asking for a
contract nobody implements cannot do anything useful, and a stack trace at boot
beats a nil dereference three screens in. Use `TryUse` when the dependency is
genuinely optional.

**Announce**, when you do not:

```go
eventbus.Publish(s.bus, ctx, notesapi.Deleted{ID: id, Title: n.Title})
```

The publisher does not know who is listening, and does not care. Whether that
event redraws a badge, writes an audit row, or falls on an empty room is not the
notes module's problem.

Events are facts, so they are published *after* the thing happened, never before.
A create that failed publishes nothing.

Delivery is synchronous: `Publish` returns once every handler has run. In a
desktop app that is the right trade, because the alternative (events landing at
some unspecified later point) turns an ordering bug into a heisenbug. Handlers must
therefore be quick, and anything slow belongs on a goroutine the handler spawns
itself.

The `dashboard` module does both, in eighty lines: it reads note counts through
`notesapi.Service`, and refreshes itself on `notesapi.Created` and
`notesapi.Deleted`. It owns no tables and provides no service. Delete the
directory and its line in `main.go` and the feature is gone, cleanly. That is the
property all of this is for.

## GTK from Go: the thread rule

GTK is single-threaded. Every widget call has to happen on the thread running the
main loop.

Go does not know this and will not help you. A goroutine doing a database read
finishes wherever it finishes, and if the callback touches a widget from there,
GTK's internal state is quietly corrupted. The app does not crash at the offence.
It crashes later, somewhere blameless, and you spend an afternoon reading the
wrong stack trace.

So: work goes on a goroutine, results come back through the main loop.

```go
ui.Async(ctx,
    func(ctx context.Context) ([]notesapi.Note, error) {
        return svc.List(ctx)     // background: no widgets here
    },
    func(notes []notesapi.Note, err error) {
        listView.SetModel(model) // main thread: widgets are fine
    },
)
```

`ui.Async` also drops the callback if `ctx` was cancelled while the work was in
flight. The screen that asked is gone, and calling back into its widgets is how
you get a use-after-free.

Event handlers run on whichever goroutine published, which is rarely the main
thread. Anything they do to a widget goes through `ui.OnMain`.

## Storage

One SQLite file, shared by every module. Modules stay out of each other's way by
convention: a module may only touch tables prefixed with its own ID. `archlint`
cannot check that, so code review has to.

Migrations are owned by the module, keyed by `(module, name)`, and applied in
order at boot. Each commits in its own transaction, so a failure halfway through
leaves the earlier ones applied and recorded, and the next boot resumes where it
stopped instead of replaying work that already succeeded.

A shipped migration must never be edited. It is keyed by name, so a change to its
SQL will not re-run on machines that already have it, and the schema silently
diverges between users. Add another one.

The driver is `modernc.org/sqlite`, a pure-Go translation of SQLite. The C one is
faster, but this binary already links GTK through cgo, and a second C dependency
buys a cross-compilation headache for a database that holds one user's data.

## Why the UI is built in Go

The GNOME-idiomatic answer is Blueprint compiled to `.ui`, packed into a
GResource. It separates layout from logic, and it is genuinely nicer to read.

It also means `go build` alone no longer builds the project: you need
`blueprint-compiler` and `glib-compile-resources` installed and run in the right
order first. For a boilerplate whose entire promise is that one command produces
a running app, that is a bad trade. gotk4's support for composite templates and
subclassing is awkward enough that the nicer syntax does not pay for itself
either.

So the widgets are built in Go, the stylesheet is `go:embed`ed, and the whole
project builds with the Go toolchain and nothing else. If you want Blueprint
later, nothing here stops you: add the compile step to the Makefile and load the
`.ui` files with `gtk.NewBuilderFromString`.

## Testing

The domain, service and platform layers import no GTK. That is not an accident;
it is what makes them testable.

Domain tests need nothing at all. Service tests use a real SQLite database, in
memory, created fresh per test: mocking the store would test the mock, whereas
this tests the SQL, the migrations and the service together, and still runs in
milliseconds. Neither needs a display, so both run on a headless CI runner.

The UI is not unit-tested, and pretending otherwise would be theatre. Keep the
screens thin, so that they call a service and render what comes back, and the
logic worth testing stays where the tests can reach it.
