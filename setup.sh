#!/usr/bin/env bash
#
# setup.sh: install what the stack needs, then open the project wizard.
#
#   ./setup.sh                      open the graphical wizard
#   ./setup.sh --batch --name Foo --dir ~/src/foo
#                                   create a project with no window (for CI)
#   ./setup.sh --deps-only          install the dependencies and stop
#   ./setup.sh --check              report what is missing and stop
#
# Anything else is passed straight through to the wizard; see cmd/setup.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Go is not optional and not worth guessing about. gotk4 needs generics that
# landed in 1.21, and the template's go.mod asks for 1.25.
readonly GO_MIN_MAJOR=1
readonly GO_MIN_MINOR=25

# ---------------------------------------------------------------------------
# output
# ---------------------------------------------------------------------------

if [[ -t 1 ]]; then
    BOLD=$'\033[1m'; RED=$'\033[31m'; GREEN=$'\033[32m'; YELLOW=$'\033[33m'; DIM=$'\033[2m'; RESET=$'\033[0m'
else
    BOLD=''; RED=''; GREEN=''; YELLOW=''; DIM=''; RESET=''
fi

step()  { printf '%s==>%s %s\n' "$BOLD" "$RESET" "$*"; }
ok()    { printf '  %s✓%s %s\n' "$GREEN" "$RESET" "$*"; }
warn()  { printf '  %s!%s %s\n' "$YELLOW" "$RESET" "$*"; }
err()   { printf '  %s✗%s %s\n' "$RED" "$RESET" "$*" >&2; }
note()  { printf '  %s%s%s\n' "$DIM" "$*" "$RESET"; }

die() { err "$*"; exit 1; }

# ---------------------------------------------------------------------------
# system dependencies
# ---------------------------------------------------------------------------

# The pkg-config names gotk4 links against. These are what actually decide
# whether the build works, so the check is on them rather than on package names,
# which differ across every distribution.
readonly REQUIRED_PC=(
    gtk4
    libadwaita-1
    glib-2.0
    gobject-introspection-1.0
)

detect_distro() {
    if [[ -r /etc/os-release ]]; then
        # shellcheck disable=SC1091
        . /etc/os-release
        printf '%s' "${ID_LIKE:-$ID}"
        return
    fi
    printf 'unknown'
}

packages_for() {
    case "$1" in
        *debian*|*ubuntu*)
            printf '%s\n' build-essential pkg-config libgtk-4-dev libadwaita-1-dev \
                          libgirepository1.0-dev libglib2.0-dev-bin
            ;;
        *fedora*|*rhel*)
            printf '%s\n' gcc pkgconf-pkg-config gtk4-devel libadwaita-devel \
                          gobject-introspection-devel glib2-devel
            ;;
        *arch*)
            printf '%s\n' base-devel pkgconf gtk4 libadwaita gobject-introspection glib2
            ;;
        *suse*)
            printf '%s\n' gcc pkg-config gtk4-devel libadwaita-devel \
                          gobject-introspection-devel glib2-devel
            ;;
        *)
            return 1
            ;;
    esac
}

install_command_for() {
    case "$1" in
        *debian*|*ubuntu*) printf 'sudo apt-get install -y' ;;
        *fedora*|*rhel*)   printf 'sudo dnf install -y' ;;
        *arch*)            printf 'sudo pacman -S --needed --noconfirm' ;;
        *suse*)            printf 'sudo zypper install -y' ;;
        *)                 return 1 ;;
    esac
}

missing_pc() {
    local missing=()
    for pc in "${REQUIRED_PC[@]}"; do
        if ! pkg-config --exists "$pc" 2>/dev/null; then
            missing+=("$pc")
        fi
    done
    printf '%s\n' "${missing[@]:-}"
}

install_system_deps() {
    step "Checking GTK4 and libadwaita"

    if ! command -v pkg-config >/dev/null 2>&1; then
        warn "pkg-config is missing, so nothing can be checked yet"
    else
        local missing
        mapfile -t missing < <(missing_pc)
        # mapfile on empty input still yields one empty element.
        if [[ ${#missing[@]} -eq 0 || -z ${missing[0]} ]]; then
            for pc in "${REQUIRED_PC[@]}"; do
                ok "$pc $(pkg-config --modversion "$pc")"
            done
            return 0
        fi
        for pc in "${missing[@]}"; do
            warn "$pc is missing"
        done
    fi

    local distro packages install_cmd
    distro="$(detect_distro)"

    if ! packages="$(packages_for "$distro")" || ! install_cmd="$(install_command_for "$distro")"; then
        err "I do not know how to install packages on this system ($distro)."
        note "Install the development files for GTK4, libadwaita, GLib and"
        note "gobject-introspection using your package manager, then run this again."
        exit 1
    fi

    # shellcheck disable=SC2206
    local pkg_list=($packages)

    step "Installing system packages"
    note "$install_cmd ${pkg_list[*]}"
    echo

    # sudo is asked for here, out loud, rather than sprung on the user in the
    # middle of a build.
    if ! $install_cmd "${pkg_list[@]}"; then
        die "package installation failed"
    fi

    local still_missing
    mapfile -t still_missing < <(missing_pc)
    if [[ ${#still_missing[@]} -gt 0 && -n ${still_missing[0]} ]]; then
        die "still missing after install: ${still_missing[*]}"
    fi

    ok "GTK4 and libadwaita are ready"
}

# ---------------------------------------------------------------------------
# Go
# ---------------------------------------------------------------------------

go_version_ok() {
    local version major minor
    version="$(go env GOVERSION 2>/dev/null || true)"   # e.g. go1.26.5
    version="${version#go}"
    major="${version%%.*}"
    minor="${version#*.}"
    minor="${minor%%.*}"

    [[ -n "$major" && -n "$minor" ]] || return 1
    (( major > GO_MIN_MAJOR )) && return 0
    (( major == GO_MIN_MAJOR && minor >= GO_MIN_MINOR )) && return 0
    return 1
}

# load_gvm puts a gvm-managed Go on the PATH, if there is one and it is not there
# already.
#
# gvm only edits the PATH of an interactive login shell, which is not the shell a
# script runs in. Skip this and someone whose Go came from gvm gets told Go is not
# installed, which is both wrong and unhelpful.
#
# Two things about gvm, both of which bite under `set -euo pipefail`.
#
# First, its environment scripts read variables such as PKG_CONFIG_PATH and
# GVM_DEBUG without checking whether they are set, so sourcing gvm under `set -u`
# kills the script with "unbound variable" before it has done anything. Relax the
# rule around the source, then put it back.
#
# Second, and less obviously, gvm replaces the `cd` builtin with a shell function
# that switches Go version per directory. That function has the same
# unbound-variable problem, so the next `cd` anywhere in this script would blow up
# too. Setting a default for every variable it happens to read today is
# whack-a-mole. Dropping the overrides is not: this script has no use for
# per-directory Go switching, and without them `cd` is the builtin again.
load_gvm() {
    command -v go >/dev/null 2>&1 && return 0
    [[ -s "$HOME/.gvm/scripts/gvm" ]] || return 0

    set +eu
    # shellcheck disable=SC1091
    source "$HOME/.gvm/scripts/gvm"
    for override in cd pushd popd; do
        unset -f "$override" 2>/dev/null
    done
    set -eu
}

ensure_go() {
    step "Checking Go"

    load_gvm

    if command -v go >/dev/null 2>&1 && go_version_ok; then
        ok "$(go version)"
        return 0
    fi

    if command -v go >/dev/null 2>&1; then
        err "Go $(go env GOVERSION) is too old; ${GO_MIN_MAJOR}.${GO_MIN_MINOR} or newer is needed."
    else
        err "Go is not installed."
    fi

    if command -v gvm >/dev/null 2>&1; then
        note "You have gvm. Install a supported Go and make it the default:"
        note "  gvm install go1.26.5 -B && gvm use go1.26.5 --default"
    else
        note "Install it from https://go.dev/dl/, or with your package manager."
    fi
    exit 1
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

main() {
    local deps_only=false check_only=false
    local passthrough=()

    for arg in "$@"; do
        case "$arg" in
            --deps-only) deps_only=true ;;
            --check)     check_only=true ;;
            *)           passthrough+=("$arg") ;;
        esac
    done

    printf '%sJoinery: GTK4 + Go project starter%s\n\n' "$BOLD" "$RESET"

    if $check_only; then
        step "Checking dependencies"

        local missing
        mapfile -t missing < <(missing_pc)
        if [[ ${#missing[@]} -eq 0 || -z ${missing[0]} ]]; then
            ok "all system libraries present"
        else
            for pc in "${missing[@]}"; do warn "$pc is missing"; done
        fi

        # Look where Go actually lives before reporting it missing. A gvm-managed
        # Go is not on a script's PATH until gvm is sourced.
        load_gvm
        if command -v go >/dev/null 2>&1; then
            ok "$(go version)"
        else
            warn "Go is not installed"
        fi
        exit 0
    fi

    install_system_deps
    ensure_go

    if $deps_only; then
        ok "dependencies are ready; stopping here as asked"
        exit 0
    fi

    step "Building the wizard"
    cd "$REPO_DIR"

    # The first build compiles gotk4's bindings, which are large. Say so, or the
    # silence reads as a hang.
    note "the first build takes a few minutes: gotk4's bindings are big"
    if ! go build -o bin/setup ./cmd/setup; then
        die "could not build the wizard"
    fi
    ok "built bin/setup"

    step "Starting the wizard"

    # ${a[@]+"${a[@]}"} rather than "${a[@]}": under `set -u`, expanding an
    # empty array is an error on older bash, and "${a[@]:-}" would hand the
    # wizard a single empty argument, which flag parsing then rejects.
    exec ./bin/setup ${passthrough[@]+"${passthrough[@]}"}
}

main "$@"
