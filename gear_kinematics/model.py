"""Input model and validation for compound gear trains."""

from __future__ import annotations

from dataclasses import dataclass, field


class ModelError(ValueError):
    """Raised when the gear train definition is invalid."""


@dataclass(frozen=True)
class Connection:
    kind: str
    label: str
    shafts: tuple[str, ...]
    teeth: tuple[int, ...] = ()


@dataclass(frozen=True)
class Case:
    name: str
    speeds: dict[str, float]
    fixed: tuple[str, ...]
    torques: dict[str, float]


@dataclass(frozen=True)
class GearTrain:
    shafts: tuple[str, ...]
    connections: tuple[Connection, ...]
    cases: tuple[Case, ...] = field(default=())


def _positive_int(value, what: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int) or value <= 0:
        raise ModelError(f"{what}: teeth must be a positive integer, got {value!r}")
    return value


def _check_shaft(shafts: set[str], name, what: str) -> str:
    if not isinstance(name, str) or name not in shafts:
        raise ModelError(f"{what}: unknown shaft {name!r}")
    return name


def _parse_connection(raw: dict, shafts: set[str], index: int) -> Connection:
    if not isinstance(raw, dict):
        raise ModelError(f"connection #{index}: must be an object")
    kind = raw.get("type")
    label = raw.get("label") or f"connection #{index} ({kind})"
    if kind == "rigid":
        pair = raw.get("shafts")
        if not isinstance(pair, list) or len(pair) != 2:
            raise ModelError(f"{label}: 'shafts' must list exactly two shafts")
        a = _check_shaft(shafts, pair[0], label)
        b = _check_shaft(shafts, pair[1], label)
        if a == b:
            raise ModelError(f"{label}: rigid link needs two distinct shafts")
        return Connection(kind, label, (a, b))
    if kind in ("external_mesh", "internal_mesh"):
        g1, g2 = raw.get("gear1"), raw.get("gear2")
        for tag, g in (("gear1", g1), ("gear2", g2)):
            if not isinstance(g, dict):
                raise ModelError(f"{label}: '{tag}' must be an object")
        s1 = _check_shaft(shafts, g1.get("shaft"), f"{label} gear1")
        s2 = _check_shaft(shafts, g2.get("shaft"), f"{label} gear2")
        if s1 == s2:
            raise ModelError(f"{label}: meshing gears must be on distinct shafts")
        z1 = _positive_int(g1.get("teeth"), f"{label} gear1")
        z2 = _positive_int(g2.get("teeth"), f"{label} gear2")
        return Connection(kind, label, (s1, s2), (z1, z2))
    if kind == "planetary":
        sun, ring = raw.get("sun"), raw.get("ring")
        if not isinstance(sun, dict) or not isinstance(ring, dict):
            raise ModelError(f"{label}: 'sun' and 'ring' must be objects")
        s_sun = _check_shaft(shafts, sun.get("shaft"), f"{label} sun")
        s_ring = _check_shaft(shafts, ring.get("shaft"), f"{label} ring")
        s_carrier = _check_shaft(shafts, raw.get("carrier"), f"{label} carrier")
        if len({s_sun, s_ring, s_carrier}) != 3:
            raise ModelError(f"{label}: sun, ring and carrier must be distinct shafts")
        z_sun = _positive_int(sun.get("teeth"), f"{label} sun")
        z_ring = _positive_int(ring.get("teeth"), f"{label} ring")
        diff = z_ring - z_sun
        if diff <= 0 or diff % 2 != 0:
            raise ModelError(
                f"{label}: geometry impossible, need ring_teeth = sun_teeth + 2*planet_teeth "
                f"with positive integer planet teeth (got sun={z_sun}, ring={z_ring})"
            )
        return Connection(kind, label, (s_sun, s_ring, s_carrier), (z_sun, z_ring))
    raise ModelError(f"{label}: unknown connection type {kind!r}")


def load_train(data: dict) -> GearTrain:
    """Validate a parsed JSON document and return a GearTrain."""
    if not isinstance(data, dict):
        raise ModelError("top-level document must be an object")
    raw_shafts = data.get("shafts")
    if not isinstance(raw_shafts, list) or not raw_shafts or \
            any(not isinstance(s, str) for s in raw_shafts):
        raise ModelError("'shafts' must be a non-empty list of names")
    if len(set(raw_shafts)) != len(raw_shafts):
        raise ModelError("'shafts' contains duplicates")
    shafts = tuple(raw_shafts)
    shaft_set = set(shafts)

    raw_conns = data.get("connections", [])
    if not isinstance(raw_conns, list):
        raise ModelError("'connections' must be a list")
    connections = tuple(
        _parse_connection(c, shaft_set, i) for i, c in enumerate(raw_conns)
    )

    cases = []
    for i, raw in enumerate(data.get("cases", [])):
        if not isinstance(raw, dict):
            raise ModelError(f"case #{i}: must be an object")
        name = raw.get("name") or f"case #{i}"
        speeds = raw.get("speeds", {})
        if not isinstance(speeds, dict):
            raise ModelError(f"{name}: 'speeds' must be an object")
        for shaft, value in speeds.items():
            _check_shaft(shaft_set, shaft, f"{name} speeds")
            if isinstance(value, bool) or not isinstance(value, (int, float)):
                raise ModelError(f"{name}: speed of {shaft!r} must be a number (rpm)")
        fixed = raw.get("fixed", [])
        if not isinstance(fixed, list):
            raise ModelError(f"{name}: 'fixed' must be a list")
        fixed = tuple(_check_shaft(shaft_set, s, f"{name} fixed") for s in fixed)
        clash = set(fixed) & set(speeds)
        if clash:
            raise ModelError(f"{name}: shafts both fixed and speed-specified: {sorted(clash)}")
        torques = raw.get("torques", {})
        if not isinstance(torques, dict):
            raise ModelError(f"{name}: 'torques' must be an object")
        for shaft, value in torques.items():
            _check_shaft(shaft_set, shaft, f"{name} torques")
            if isinstance(value, bool) or not isinstance(value, (int, float)):
                raise ModelError(f"{name}: torque of {shaft!r} must be a number (N*m)")
        cases.append(Case(name, dict(speeds), fixed, dict(torques)))

    return GearTrain(shafts, connections, tuple(cases))
