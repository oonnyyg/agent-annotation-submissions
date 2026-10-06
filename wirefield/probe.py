"""Measurement points and probe orientation (local orthonormal basis)."""

from __future__ import annotations

import csv
import math
from dataclasses import dataclass

from .coil import CoilDataError
from .vector import Vec3, cross, dot, is_finite_vec, norm

BASIS_TOLERANCE = 1e-6

COLUMNS = (
    "x",
    "y",
    "z",
    "u1",
    "u2",
    "u3",
    "v1",
    "v2",
    "v3",
    "w1",
    "w2",
    "w3",
)


@dataclass(frozen=True)
class Probe:
    """A measurement point with a right-handed orthonormal probe basis.

    axis_u/axis_v/axis_w are the probe's three local axes expressed in
    world coordinates. Local field components are plain dot products, so
    the field magnitude is invariant under probe rotation.
    """

    point: Vec3
    axis_u: Vec3
    axis_v: Vec3
    axis_w: Vec3

    def local_components(self, world: Vec3) -> Vec3:
        return (
            dot(world, self.axis_u),
            dot(world, self.axis_v),
            dot(world, self.axis_w),
        )


def validate_basis(u: Vec3, v: Vec3, w: Vec3, label: str) -> None:
    for name, axis in (("u", u), ("v", v), ("w", w)):
        if not is_finite_vec(axis):
            raise CoilDataError(f"{label}: axis {name} has non-finite component")
        if abs(norm(axis) - 1.0) > BASIS_TOLERANCE:
            raise CoilDataError(f"{label}: axis {name} is not a unit vector")
    for n1, a, n2, b in (("u", u, "v", v), ("u", u, "w", w), ("v", v, "w", w)):
        if abs(dot(a, b)) > BASIS_TOLERANCE:
            raise CoilDataError(f"{label}: axes {n1} and {n2} are not orthogonal")
    if dot(cross(u, v), w) < 1.0 - 1e-3:
        raise CoilDataError(f"{label}: axes must form a right-handed basis (u x v = w)")


def _parse_float(raw: str, label: str, column: str) -> float:
    try:
        value = float(raw)
    except (TypeError, ValueError):
        raise CoilDataError(f"{label}: column {column!r} is not a number") from None
    if not math.isfinite(value):
        raise CoilDataError(f"{label}: column {column!r} is not finite")
    return value


def parse_probe_row(row: dict, line: int) -> Probe:
    label = f"probe row {line}"
    missing = [c for c in COLUMNS if row.get(c) in (None, "")]
    if missing:
        raise CoilDataError(f"{label}: missing columns {missing}")
    values = {c: _parse_float(row[c], label, c) for c in COLUMNS}
    point = (values["x"], values["y"], values["z"])
    u = (values["u1"], values["u2"], values["u3"])
    v = (values["v1"], values["v2"], values["v3"])
    w = (values["w1"], values["w2"], values["w3"])
    validate_basis(u, v, w, label)
    return Probe(point=point, axis_u=u, axis_v=v, axis_w=w)


def load_probes(path: str) -> list[Probe]:
    probes: list[Probe] = []
    with open(path, newline="", encoding="utf-8-sig") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames is None:
            raise CoilDataError("probe CSV is empty")
        missing = [c for c in COLUMNS if c not in reader.fieldnames]
        if missing:
            raise CoilDataError(f"probe CSV missing header columns {missing}")
        for line, row in enumerate(reader, start=2):
            probes.append(parse_probe_row(row, line))
    if not probes:
        raise CoilDataError("probe CSV contains no measurement points")
    return probes
