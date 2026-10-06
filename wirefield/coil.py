"""Closed polyline coil model and JSON validation."""

from __future__ import annotations

import math
from dataclasses import dataclass

from .vector import Vec3, is_finite_vec, norm, sub

MIN_VERTICES = 3


class CoilDataError(ValueError):
    """Raised when coil or scenario input data is invalid."""


@dataclass(frozen=True)
class Coil:
    """Closed polyline current loop.

    Vertices are ordered along the current direction; the last vertex is
    implicitly connected back to the first. Current is signed (amperes).
    """

    coil_id: str
    vertices: tuple[Vec3, ...]
    current: float

    def segments(self) -> list[tuple[Vec3, Vec3]]:
        pts = list(self.vertices)
        return [(pts[i], pts[(i + 1) % len(pts)]) for i in range(len(pts))]


def _parse_vertex(raw: object, coil_id: str, index: int) -> Vec3:
    ok = isinstance(raw, (list, tuple)) and len(raw) == 3
    if ok:
        ok = all(isinstance(c, (int, float)) and not isinstance(c, bool) for c in raw)
    if not ok:
        raise CoilDataError(
            f"coil {coil_id!r} vertex {index}: expected a list of 3 numbers (metres)"
        )
    vec = (float(raw[0]), float(raw[1]), float(raw[2]))
    if not is_finite_vec(vec):
        raise CoilDataError(f"coil {coil_id!r} vertex {index}: non-finite coordinate")
    return vec


def parse_coil(raw: object, index: int) -> Coil:
    if not isinstance(raw, dict):
        raise CoilDataError(f"coil #{index}: expected an object")
    coil_id = raw.get("id", f"coil{index}")
    if not isinstance(coil_id, str) or not coil_id:
        raise CoilDataError(f"coil #{index}: 'id' must be a non-empty string")

    raw_vertices = raw.get("vertices")
    if not isinstance(raw_vertices, list):
        raise CoilDataError(f"coil {coil_id!r}: 'vertices' must be a list")
    if len(raw_vertices) < MIN_VERTICES:
        raise CoilDataError(
            f"coil {coil_id!r}: at least {MIN_VERTICES} vertices required"
        )
    vertices = tuple(_parse_vertex(v, coil_id, i) for i, v in enumerate(raw_vertices))

    current = raw.get("current")
    if isinstance(current, bool) or not isinstance(current, (int, float)):
        raise CoilDataError(f"coil {coil_id!r}: 'current' must be a number (amperes)")
    current = float(current)
    if not math.isfinite(current):
        raise CoilDataError(f"coil {coil_id!r}: current must be finite")

    coil = Coil(coil_id=coil_id, vertices=vertices, current=current)
    for a, b in coil.segments():
        if norm(sub(b, a)) == 0.0:
            raise CoilDataError(
                f"coil {coil_id!r}: zero-length edge at vertex {a!r} "
                "(consecutive duplicate vertices, including last-to-first)"
            )
    return coil


def _parse_scenario(raw: object, index: int, coil_ids: list[str]) -> dict:
    if not isinstance(raw, dict):
        raise CoilDataError(f"scenario #{index}: expected an object")
    name = raw.get("name", f"scenario{index}")
    if not isinstance(name, str) or not name:
        raise CoilDataError(f"scenario #{index}: 'name' must be a non-empty string")
    currents = raw.get("currents")
    if not isinstance(currents, dict) or not currents:
        raise CoilDataError(
            f"scenario {name!r}: 'currents' must map coil id to amperes"
        )
    parsed: dict[str, float] = {}
    for cid, value in currents.items():
        if cid not in coil_ids:
            raise CoilDataError(f"scenario {name!r}: unknown coil id {cid!r}")
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise CoilDataError(
                f"scenario {name!r}: current for coil {cid!r} must be a number"
            )
        if not math.isfinite(float(value)):
            raise CoilDataError(
                f"scenario {name!r}: current for coil {cid!r} must be finite"
            )
        parsed[cid] = float(value)
    missing = [cid for cid in coil_ids if cid not in parsed]
    if missing:
        raise CoilDataError(f"scenario {name!r}: missing currents for coils {missing}")
    return {"name": name, "currents": parsed}


def load_config(raw: object) -> tuple[list[Coil], list[dict]]:
    """Parse the top-level JSON config into (coils, scenarios)."""
    if not isinstance(raw, dict):
        raise CoilDataError("config root must be an object")
    raw_coils = raw.get("coils")
    if not isinstance(raw_coils, list) or not raw_coils:
        raise CoilDataError("config must contain a non-empty 'coils' list")
    coils = [parse_coil(c, i) for i, c in enumerate(raw_coils)]
    ids = [c.coil_id for c in coils]
    if len(set(ids)) != len(ids):
        raise CoilDataError("coil ids must be unique")

    raw_scenarios = raw.get("scenarios")
    scenarios: list[dict] = []
    if raw_scenarios is not None:
        if not isinstance(raw_scenarios, list) or not raw_scenarios:
            raise CoilDataError("'scenarios' must be a non-empty list when present")
        for i, sc in enumerate(raw_scenarios):
            scenarios.append(_parse_scenario(sc, i, ids))
    return coils, scenarios
