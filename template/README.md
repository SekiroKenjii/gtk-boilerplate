# GtkApp

A Linux desktop application: Go on the inside, GTK4 and libadwaita on the
outside, arranged as a modular monolith.

## Running it

```sh
make run
```

`make` on its own lists everything else.

## Layout

```text
cmd/app/                 the composition root: the only file that knows every module
internal/
  app/                   the kernel: the Module contract, the service registry, the window shell
  platform/              what every module may use: config, logging, database, event bus, GTK helpers
  modules/
    notes/               an example feature, as a full vertical slice
    settings/            user preferences
    dashboard/           an example of a module built only from other modules' contracts
tools/archlint/          the boundary checker; `make arch` runs it
build/                   desktop entry, AppStream metadata, Flatpak manifest
```

## The one rule

A module may reach another module **only through its `api` package**.

```text
internal/modules/notes/ui  ->  internal/modules/settings/api        yes
internal/modules/notes/ui  ->  internal/modules/settings/service    no
```

Everything else (the domain, the store, the service, the screens) is private to
the module that owns it. `make arch` fails the build when that is broken, so the
rule survives contact with a deadline.

The payoff: a feature is a directory plus one line in `cmd/app/main.go`.
Deleting it is the same edit in reverse.

## Adding a module

1. Copy `internal/modules/notes/` to `internal/modules/yours/`.
2. Write your contract in `api/`: interfaces, data, events. Nothing else.
3. Publish your service from `Register`, resolve other modules' from `Start`.
4. Mount it in `cmd/app/main.go`.
5. `make arch` to check you have not reached through anything.

## Two ways for modules to talk

**Ask for something.** Resolve the other module's contract from the kernel:

```go
store := app.Use[settingsapi.Store](k)
if store.Get().ConfirmDelete { ... }
```

**Announce something.** Publish an event and forget who is listening:

```go
eventbus.Publish(s.bus, ctx, notesapi.Deleted{ID: id, Title: n.Title})
```

Use the first when you need an answer, the second when you do not. The dashboard
module does both, in about eighty lines, if you want to see it.

## GTK from Go: the one thing to get right

GTK is single-threaded. Every widget call has to happen on the main thread, and
Go will happily run your code somewhere else. Touching a widget from a
background goroutine does not crash reliably. It corrupts GTK's state, and the
app falls over later, somewhere innocent.

So work goes on a goroutine, and the result comes back through `ui.Async`:

```go
ui.Async(ctx,
    func(ctx context.Context) ([]notesapi.Note, error) { return svc.List(ctx) },
    func(notes []notesapi.Note, err error) {
        // back on the main thread; safe to touch widgets
    },
)
```

## Testing

```sh
make test
```

The domain, service and platform layers import no GTK, on purpose: those tests
need no display and run on any CI box. The service tests use a real SQLite
database, in memory, created fresh per test.

## Packaging

```sh
make install    # into ~/.local, so the app appears in your launcher
make flatpak    # a Flatpak bundle
```

Before publishing, replace the placeholder icon in `assets/`, fill in the
screenshots and URLs in `build/desktop/`, and validate the metadata:

```sh
appstreamcli validate build/desktop/com.example.GtkApp.metainfo.xml
```
