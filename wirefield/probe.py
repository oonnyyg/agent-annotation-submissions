"""Measurement points and probe orientation, loaded from CSV."""

from __future__ import annotations

import csv
import math
from dataclasses import dataclass
from pathlib import Path

from .vector import Vec3, cross, dot, is_finite, norm

_ORTH_TOL = 1e-6


class ProbeDataError(ValueError):
    """Raised when a probe CSV file fails validation."""


@dataclass(frozen=True)
class Probe:
    name: str
    position: Vec3
    axis_x: Vec3
    axis_y: Vec3
    axis_z: Vec3

    def to_local(self, world: Vec3) -> Vec3:
        """Project a world vector onto the probe's local axes."""
        return (
            dot(world, self.axis_x),
            dot(world, self.axis_y),
            dot(world, self.axis_z),
        )


def _check_basis(x: Vec3, y: Vec3, z: Vec3, where: str) -> None:
    for label, axis in (("x", x), ("y", y), ("z", z)):
        if abs(norm(axis) - 1.0) > _ORTH_TOL:
            raise ProbeDataError(f"{where}: axis {label} is not a unit vector")
    if abs(dot(x, y)) > _ORTH_TOL or abs(dot(y, z)) > _ORTH_TOL or abs(dot(z, x)) > _ORTH_TOL:
        raise ProbeDataError(f"{where}: axes are not mutually orthogonal")
    handed = dot(cross(x, y), z)
    if handed <= 0.0:
        raise ProbeDataError(f"{where}: axes must form a right-handed basis")


def _float(row: dict[str, str], key: str, where: str) -> float:
    raw = row.get(key)
    if raw is None or raw.strip() == "":
        raise ProbeDataError(f"{where}: missing column '{key}'")
    try:
        value = float(raw)
    except ValueError:
        raise ProbeDataError(f"{where}: '{key}' is not a number: {raw!r}") from None
    if not math.isfinite(value):
        raise ProbeDataError(f"{where}: '{key}' must be finite")
    return value


def load_probes(path: str | Path) -> list[Probe]:
    text = Path(path).read_text(encoding="utf-8")
    rows = list(csv.DictReader(text.splitlines()))
    if not rows:
        raise ProbeDataError("probe CSV has no data rows")
    probes = []
    for i, row in enumerate(rows):
        where = f"row {i + 2}"
        name = (row.get("name") or f"probe_{i}").strip()
        pos = (
            _float(row, "px", where),
            _float(row, "py", where),
            _float(row, "pz", where),
        )
        axes = []
        for label in ("x", "y", "z"):
            axes.append(
                (
                    _float(row, f"{label}x", where),
                    _float(row, f"{label}y", where),
                    _float(row, f"{label}z", where),
                )
            )
        for axis in axes:
            if not is_finite(axis):
                raise ProbeDataError(f"{where}: axis components must be finite")
        _check_basis(axes[0], axes[1], axes[2], where)
        probes.append(Probe(name=name, position=pos, axis_x=axes[0], axis_y=axes[1], axis_z=axes[2]))
    return probes
