#!/usr/bin/env python3
"""Gera os ícones estáticos do PWA. Sem dependências externas."""

import struct
import zlib
from pathlib import Path


def chunk(tag: bytes, data: bytes) -> bytes:
    return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)


def write_png(path: Path, size: int, padding: int) -> None:
    raw = bytearray()
    for y in range(size):
        raw.append(0)
        for x in range(size):
            raw.extend(pixel(x, y, size, padding))
    ihdr = struct.pack(">IIBBBBB", size, size, 8, 6, 0, 0, 0)
    png = b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", ihdr) + chunk(b"IDAT", zlib.compress(bytes(raw), 9)) + chunk(b"IEND", b"")
    path.write_bytes(png)


def pixel(x: int, y: int, size: int, padding: int) -> bytes:
    hull = (16, 33, 28, 255)
    if x < padding or y < padding or x >= size - padding or y >= size - padding:
        return hull
    inner_x = x - padding
    inner_y = y - padding
    inner = size - 2 * padding
    staff_left = int(inner * 0.38)
    staff_right = int(inner * 0.62)
    if staff_left <= inner_x <= staff_right:
        level = int(inner * 0.72)
        if inner_y >= inner - level:
            return (47, 143, 120, 255)
        if abs(inner_y - int(inner * 0.28)) <= max(1, size // 128):
            return (230, 211, 163, 255)
        return (12, 25, 22, 255)
    return hull


def main() -> None:
    public = Path(__file__).resolve().parents[1] / "public"
    public.mkdir(parents=True, exist_ok=True)
    write_png(public / "icon-192.png", 192, 0)
    write_png(public / "icon-512.png", 512, 0)
    write_png(public / "icon-maskable-512.png", 512, 64)


if __name__ == "__main__":
    main()
