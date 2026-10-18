#!/usr/bin/env python3
"""Draws Polyfin's logo, assets/polyfin.svg, as the packages' icons.

Usage: icons.py png <size> <file>      a PNG, size pixels wide
       icons.py iconset <folder>       the PNGs of a macOS .iconset
       icons.py ico <file>             a Windows .ico, 16 to 256 pixels

The logo's triangles fill the icon with a margin, as the tray's icon draws
them (cmd/polyfin-desktop/icon.go), anti-aliased. Python's standard
library only: it runs on every runner.
"""
import os
import re
import struct
import sys
import zlib

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
SUBROWS = 8  # vertical samples per pixel row


def triangles():
    svg = open(os.path.join(ROOT, "assets", "polyfin.svg"), encoding="utf-8").read()
    found = []
    for points, fill in re.findall(r'<polygon points="([^"]+)" fill="#([0-9A-Fa-f]{6})"', svg):
        corners = [tuple(float(v) for v in p.split(",")) for p in points.split()]
        found.append((corners, tuple(int(fill[i:i + 2], 16) / 255 for i in (0, 2, 4))))
    if len(found) != 5:
        sys.exit("assets/polyfin.svg: expected the logo's 5 triangles, found %d" % len(found))
    return found


def coverage(corners, size, y):
    """The share of each pixel of row y the triangle covers."""
    row = [0.0] * size
    edges = list(zip(corners, corners[1:] + corners[:1]))
    for s in range(SUBROWS):
        sy = y + (s + 0.5) / SUBROWS
        xs = []
        for (x0, y0), (x1, y1) in edges:
            if (y0 <= sy < y1) or (y1 <= sy < y0):
                xs.append(x0 + (sy - y0) * (x1 - x0) / (y1 - y0))
        if len(xs) != 2:
            continue
        left, right = max(min(xs), 0.0), min(max(xs), float(size))
        if right <= left:
            continue
        first, last = int(left), min(int(right), size - 1)
        weight = 1.0 / SUBROWS
        if first == last:
            row[first] += (right - left) * weight
            continue
        row[first] += (first + 1 - left) * weight
        for x in range(first + 1, last):
            row[x] += weight
        row[last] += (right - last) * weight
    return row


def draw(size):
    """The logo, size pixels wide, as rows of RGBA bytes."""
    # As icon.go: the triangles span 15 to 85 of the 100 by 100 box.
    scale, offset = size / 76.0, 12.0
    shapes = [([((x - offset) * scale, (y - offset) * scale) for x, y in corners], color)
              for corners, color in triangles()]
    rows = []
    for y in range(size):
        # Premultiplied color, composited triangle over triangle.
        r, g, b, a = [0.0] * size, [0.0] * size, [0.0] * size, [0.0] * size
        for corners, (cr, cg, cb) in shapes:
            if not min(p[1] for p in corners) <= y + 1 or not y <= max(p[1] for p in corners):
                continue
            for x, c in enumerate(coverage(corners, size, y)):
                if c <= 0:
                    continue
                c = min(c, 1.0)
                r[x] = cr * c + r[x] * (1 - c)
                g[x] = cg * c + g[x] * (1 - c)
                b[x] = cb * c + b[x] * (1 - c)
                a[x] = c + a[x] * (1 - c)
        line = bytearray()
        for x in range(size):
            alpha = a[x]
            if alpha <= 0:
                line += b"\0\0\0\0"
                continue
            line += bytes((round(r[x] / alpha * 255), round(g[x] / alpha * 255), round(b[x] / alpha * 255),
                           round(alpha * 255)))
        rows.append(bytes(line))
    return rows


def png(size):
    rows = draw(size)
    raw = b"".join(b"\0" + row for row in rows)

    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data) & 0xFFFFFFFF)

    return (b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0))
            + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b""))


def write(path, data):
    with open(path, "wb") as f:
        f.write(data)


def main():
    kind = sys.argv[1] if len(sys.argv) > 1 else ""
    if kind == "png" and len(sys.argv) == 4:
        write(sys.argv[3], png(int(sys.argv[2])))
    elif kind == "iconset" and len(sys.argv) == 3:
        os.makedirs(sys.argv[2], exist_ok=True)
        made = {}
        for points in (16, 32, 128, 256, 512):
            for factor, suffix in ((1, ""), (2, "@2x")):
                size = points * factor
                made.setdefault(size, png(size))
                write(os.path.join(sys.argv[2], "icon_%dx%d%s.png" % (points, points, suffix)), made[size])
    elif kind == "ico" and len(sys.argv) == 3:
        # PNG images in an ICO, which Windows Vista and later read.
        images = [(size, png(size)) for size in (16, 24, 32, 48, 64, 128, 256)]
        header = struct.pack("<HHH", 0, 1, len(images))
        offset = len(header) + 16 * len(images)
        entries, data = b"", b""
        for size, image in images:
            entries += struct.pack("<BBBBHHII", size % 256, size % 256, 0, 0, 1, 32, len(image), offset + len(data))
            data += image
        write(sys.argv[2], header + entries + data)
    else:
        sys.exit(__doc__)


if __name__ == "__main__":
    main()
