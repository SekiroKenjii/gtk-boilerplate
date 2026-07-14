# GtkApp — build, test, package.
#
# Run `make` on its own to see what is available.

APP_ID      := com.example.GtkApp
BINARY      := gtkapp
MODULE      := github.com/example/gtkapp

VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo 0.1.0-dev)
COMMIT      ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE        ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

BIN_DIR     := bin
PREFIX      ?= $(HOME)/.local

# Version metadata is stamped into the binary rather than read from a file at
# runtime, so a built artifact can always say exactly where it came from.
LDFLAGS := -X '$(MODULE)/internal/app.Version=$(VERSION)' \
           -X '$(MODULE)/internal/app.Commit=$(COMMIT)' \
           -X '$(MODULE)/internal/app.Date=$(DATE)'

.DEFAULT_GOAL := help

## help: list the targets
help:
	@echo "$(APP_ID) — $(VERSION)"
	@echo
	@sed -n 's/^## //p' $(MAKEFILE_LIST) | awk -F': ' '{printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

## build: compile the application into bin/
build:
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) ./cmd/app
	@echo "built $(BIN_DIR)/$(BINARY) ($(VERSION))"

## run: build and launch
run: build
	./$(BIN_DIR)/$(BINARY)

## debug: launch with debug logging and GTK's inspector
#
# Ctrl-Shift-D opens the inspector: it lets you walk the widget tree and poke at
# CSS live, which beats guessing why a box is the wrong size.
debug: build
	APP_LOG_LEVEL=debug GTK_DEBUG=interactive ./$(BIN_DIR)/$(BINARY)

## test: run the unit tests
#
# None of them need a display: the domain, service and platform layers are free
# of GTK on purpose, so this runs on a headless CI box.
test:
	go test ./internal/...

## cover: run the tests and open a coverage report
cover:
	go test -coverprofile=coverage.out ./internal/...
	go tool cover -html=coverage.out

## lint: vet, formatting check, and the architecture boundaries
lint: vet fmt-check arch

vet:
	go vet ./...

fmt-check:
	@unformatted=$$(gofmt -l cmd internal tools); \
	if [ -n "$$unformatted" ]; then \
		echo "these files are not gofmt'd:"; echo "$$unformatted"; exit 1; \
	fi

## arch: check the module boundaries hold
#
# This is the one that keeps the monolith modular. See tools/archlint.
arch:
	@go run ./tools/archlint

## fmt: format the code
fmt:
	gofmt -w cmd internal tools

## tidy: sync go.mod with the imports
tidy:
	go mod tidy

## install: install the binary, desktop entry and icon into ~/.local
#
# This makes the app show up in your desktop's launcher. It does not need root:
# ~/.local/share is on the XDG data path already.
install: build
	install -Dm755 $(BIN_DIR)/$(BINARY) $(PREFIX)/bin/$(BINARY)
	install -Dm644 build/desktop/$(APP_ID).desktop $(PREFIX)/share/applications/$(APP_ID).desktop
	install -Dm644 build/desktop/$(APP_ID).metainfo.xml $(PREFIX)/share/metainfo/$(APP_ID).metainfo.xml
	install -Dm644 assets/$(APP_ID).svg $(PREFIX)/share/icons/hicolor/scalable/apps/$(APP_ID).svg
	-update-desktop-database $(PREFIX)/share/applications 2>/dev/null
	-gtk-update-icon-cache -f -t $(PREFIX)/share/icons/hicolor 2>/dev/null
	@echo "installed. it should now appear in your launcher."

## uninstall: remove what install put down
uninstall:
	rm -f $(PREFIX)/bin/$(BINARY)
	rm -f $(PREFIX)/share/applications/$(APP_ID).desktop
	rm -f $(PREFIX)/share/metainfo/$(APP_ID).metainfo.xml
	rm -f $(PREFIX)/share/icons/hicolor/scalable/apps/$(APP_ID).svg

## flatpak: build and install a Flatpak bundle
#
# Needs flatpak-builder and the GNOME SDK:
#   flatpak install flathub org.gnome.Platform//48 org.gnome.Sdk//48 org.freedesktop.Sdk.Extension.golang//24.08
flatpak:
	flatpak-builder --force-clean --user --install build/flatpak-build \
		build/flatpak/$(APP_ID).yml

## clean: remove build artefacts
clean:
	rm -rf $(BIN_DIR) build/flatpak-build .flatpak-builder coverage.out

.PHONY: help build run debug test cover lint vet fmt-check arch fmt tidy install uninstall flatpak clean
