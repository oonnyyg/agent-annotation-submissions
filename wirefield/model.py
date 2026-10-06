"""Coil definitions, JSON loading and validation."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass
from pathlib import Path

from .vector import Vec3, is_finite, norm, sub


class CoilDataError(ValueError):
    """Raised when a coil JSON file fails validation."""


@dataclass(frozen=True)
class Coil:
    name: str
    vertices: tuple[Vec3, ...]  # ordered along current direction
    current: float  # signed amperes

    def segments(self) -> list[tuple[Vec3, Vec3]]:
        segs = []
        n = len(self.vertices)
        for i in range(n):
            segs.append((self.vertices[i], self.vertices[(i + 1) % n]))
        return segs


def _parse_vertex(raw: object, where: str) -> Vec3:
    if (
        not isinstance(raw, (list, tuple))
        or len(raw) != 3
        or not all(isinstance(c, (int, float)) and not isinstance(c, bool) for c in raw)
    ):
        raise CoilDataError(f"{where}: vertex must be a list of 3 numbers")
    vec = (float(raw[0]), float(raw[1]), float(raw[2]))
    if not is_finite(vec):
        raise CoilDataError(f"{where}: vertex coordinates must be finite")
    return vec


def _parse_coil(raw: object, index: int) -> Coil:
    where = f"coils[{index}]"
    if not isinstance(raw, dict):
        raise CoilDataError(f"{where}: must be an object")
    name = raw.get("name", f"coil_{index}")
    if not isinstance(name, str) or not name:
        raise CoilDataError(f"{where}: name must be a non-empty string")
    current = raw.get("current")
    if (
        not isinstance(current, (int, float))
        or isinstance(current, bool)
        or not math.isfinite(current)
    ):
        raise CoilDataError(f"{where}: current must be a finite number (amperes)")
    verts_raw = raw.get("vertices")
    if not isinstance(verts_raw, list):
        raise CoilDataError(f"{where}: vertices must be a list")
    if len(verts_raw) < 3:
        raise CoilDataError(f"{where}: at least 3 vertices required")
    vertices = tuple(_parse_vertex(v, f"{where}.vertices[{i}]") for i, v in enumerate(verts_raw))
    coil = Coil(name=name, vertices=vertices, current=float(current))
    for i, (a, b) in enumerate(coil.segments()):
        if norm(sub(b, a)) == 0.0:
            raise CoilDataError(f"{where}: zero-length edge at vertex {i}")
    return coil


def load_coils(path: str | Path) -> list[Coil]:
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(data, dict) or not isinstance(data.get("coils"), list):
        raise CoilDataError("top level must be an object with a 'coils' list")
    if not data["coils"]:
        raise CoilDataError("'coils' must not be empty")
    return [_parse_coil(c, i) for i, c in enumerate(data["coils"])]
