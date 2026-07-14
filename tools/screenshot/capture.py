#!/usr/bin/env python3
"""Capture an X11 window to a PNG, trimming the compositor's shadow.

Called by screenshot.sh. See that file for why this exists at all.
"""
import ctypes
import ctypes.util
import re
import subprocess
import sys

from PIL import Image

ZPixmap = 2
AllPlanes = (1 << 64) - 1

# How dark a pixel has to be to count as shadow rather than window. The Adwaita
# dark window background is around #242424, so pure black is a safe threshold:
# nothing inside the window is this dark.
SHADOW_MAX = 12


class XImage(ctypes.Structure):
    _fields_ = [
        ("width", ctypes.c_int),
        ("height", ctypes.c_int),
        ("xoffset", ctypes.c_int),
        ("format", ctypes.c_int),
        ("data", ctypes.c_void_p),
        ("byte_order", ctypes.c_int),
        ("bitmap_unit", ctypes.c_int),
        ("bitmap_bit_order", ctypes.c_int),
        ("bitmap_pad", ctypes.c_int),
        ("depth", ctypes.c_int),
        ("bytes_per_line", ctypes.c_int),
        ("bits_per_pixel", ctypes.c_int),
        ("red_mask", ctypes.c_ulong),
        ("green_mask", ctypes.c_ulong),
        ("blue_mask", ctypes.c_ulong),
    ]


def find_window(title):
    """Return (window_id, width, height) for the first window matching title."""
    tree = subprocess.run(
        ["xwininfo", "-root", "-tree"], capture_output=True, text=True, check=True
    ).stdout

    for line in tree.splitlines():
        if title not in line:
            continue
        m = re.search(r"(0x[0-9a-f]+)\s+\"[^\"]*\".*?(\d+)x(\d+)\+", line)
        if m:
            return int(m.group(1), 16), int(m.group(2)), int(m.group(3))
    return None, 0, 0


def grab(wid, w, h):
    x11 = ctypes.CDLL(ctypes.util.find_library("X11"))
    x11.XOpenDisplay.restype = ctypes.c_void_p
    x11.XGetImage.restype = ctypes.POINTER(XImage)
    x11.XGetImage.argtypes = [
        ctypes.c_void_p, ctypes.c_ulong,
        ctypes.c_int, ctypes.c_int, ctypes.c_uint, ctypes.c_uint,
        ctypes.c_ulong, ctypes.c_int,
    ]

    dpy = x11.XOpenDisplay(None)
    if not dpy:
        sys.exit("cannot open the X display")

    img = x11.XGetImage(dpy, wid, 0, 0, w, h, AllPlanes, ZPixmap)
    if not img:
        sys.exit("XGetImage failed")

    im = img.contents
    raw = ctypes.string_at(im.data, im.bytes_per_line * im.height)

    # A 32bpp X visual on a little-endian machine hands back BGRA.
    mode = "BGRX" if im.bits_per_pixel == 32 else "BGR"
    return Image.frombytes(
        "RGB", (im.width, im.height), raw, "raw", mode, im.bytes_per_line
    )


def trim_shadow(pic):
    """Make the shadow around the window transparent and crop to the window.

    A client-side-decorated GTK window reserves a margin for its drop shadow. X
    hands that margin back as opaque black, which would put a hard black frame
    around the picture on any page that is not itself black.

    Flood-filling inward from the corners is what keeps the window's own rounded
    corners intact: it only eats the black that is connected to the outside.
    """
    from PIL import ImageDraw

    pic = pic.convert("RGBA")
    w, h = pic.size

    # A sentinel no real pixel will collide with, so the fill is unambiguous.
    marker = (255, 0, 255, 0)
    for corner in ((0, 0), (w - 1, 0), (0, h - 1), (w - 1, h - 1)):
        if sum(pic.getpixel(corner)[:3]) <= SHADOW_MAX * 3:
            ImageDraw.floodfill(pic, corner, marker, thresh=SHADOW_MAX)

    pixels = pic.load()
    for y in range(h):
        for x in range(w):
            if pixels[x, y] == marker:
                pixels[x, y] = (0, 0, 0, 0)

    return pic.crop(pic.getbbox())


if __name__ == "__main__":
    title, out = sys.argv[1], sys.argv[2]

    wid, w, h = find_window(title)
    if not wid:
        sys.exit(f"no window titled {title!r} is open")

    pic = trim_shadow(grab(wid, w, h))
    pic.save(out)
    print(f"wrote {out} ({pic.width}x{pic.height})")
