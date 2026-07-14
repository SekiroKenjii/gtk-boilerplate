#!/usr/bin/env bash
#
# screenshot.sh: regenerate the wizard picture in the README.
#
#   ./tools/screenshot/screenshot.sh
#
# Run this whenever the wizard's UI changes. The image is produced by the real
# program, so it cannot drift away from what users actually see, which a
# hand-taken screenshot does the day somebody renames a label.
#
# Needs a graphical session and python3-pil. It is not part of CI.
#
# # Why it looks like this
#
# The obvious approach is to have the app render itself: GTK4 can snapshot a
# widget into a GskRenderNode and hand that to a renderer, no display server
# involved. That does not work here. gotk4 v0.3.2 binds GskRenderNode as if it
# were a GObject, which it is not, so gtk_snapshot_to_node comes back through a
# g_object_add_toggle_ref that fails its own G_IS_OBJECT assertion and the
# render node is lost.
#
# The next approach is the desktop's own screenshot service. GNOME 40 and later
# refuse org.gnome.Shell.Screenshot to anything but the shell itself, and the
# portal wants a human to click a permission dialog, which a script does not have.
#
# So the window is opened under XWayland and read straight off the X server with
# XGetImage. It is a detour, but it is a short one, and it produces an image of
# the actual widget tree rather than an approximation of it.

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
OUT="${1:-$REPO_DIR/docs/images/wizard.png}"

TITLE="New GTK Project"

# Values that make the picture readable without putting the machine it was taken
# on into a public README. They go in through the same flags a user would use, so
# the form on screen is a real run and not a mock-up.
NAME="Sound Recorder"
AUTHOR="Ada Lovelace"
DEST="/home/ada/Projects/soundrecorder"

cd "$REPO_DIR"

if ! command -v go >/dev/null 2>&1 && [[ -s "$HOME/.gvm/scripts/gvm" ]]; then
    set +eu
    # shellcheck disable=SC1091
    source "$HOME/.gvm/scripts/gvm"
    for override in cd pushd popd; do
        unset -f "$override" 2>/dev/null
    done
    set -eu
fi

echo "building the wizard"
go build -o bin/setup ./cmd/setup

echo "opening the wizard under XWayland"
# The X11 backend is the whole point: XGetImage cannot read a Wayland surface.
GDK_BACKEND=x11 ./bin/setup \
    --name "$NAME" \
    --author "$AUTHOR" \
    --dir "$DEST" >/dev/null 2>&1 &
WIZARD=$!

# Give the window time to map, lay out and finish libadwaita's opening animation.
# Grabbing it earlier catches it mid-assembly.
sleep 5

cleanup() {
    kill "$WIZARD" 2>/dev/null || true
    wait "$WIZARD" 2>/dev/null || true
}
trap cleanup EXIT

python3 "$REPO_DIR/tools/screenshot/capture.py" "$TITLE" "$OUT"
