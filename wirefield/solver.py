"""Field evaluation with per-coil contributions and safety distance."""

from __future__ import annotations

from dataclasses import dataclass

from .biot import point_segment_distance, segment_field
from .model import Coil
from .vector import Vec3, add

_ZERO: Vec3 = (0.0, 0.0, 0.0)


@dataclass(frozen=True)
class PointResult:
    """Field evaluation at one measurement point for one current scheme."""

    computable: bool
    blocking_coils: tuple[str, ...]  # coils violating the safety distance
    per_coil: tuple[Vec3, ...]  # contribution of each coil (zero if blocked)
    total: Vec3


def coils_too_close(coils: list[Coil], point: Vec3, safe_distance: float) -> list[str]:
    """Names of coils with any segment within `safe_distance` of `point`."""
    blocking = []
    for coil in coils:
        for a, b in coil.segments():
            if point_segment_distance(point, a, b) <= safe_distance:
                blocking.append(coil.name)
                break
    return blocking


def evaluate(coils: list[Coil], currents: list[float], point: Vec3, safe_distance: float) -> PointResult:
    """Superpose finite-segment Biot-Savart fields of all coils at `point`.

    If the point lies within `safe_distance` of any segment, the point is
    not computable: no field is returned (never a huge pseudo-value).
    """
    blocking = coils_too_close(coils, point, safe_distance)
    if blocking:
        return PointResult(False, tuple(blocking), tuple([_ZERO] * len(coils)), _ZERO)
    per_coil = []
    total = _ZERO
    for coil, current in zip(coils, currents):
        field = _ZERO
        for a, b in coil.segments():
            field = add(field, segment_field(point, a, b, current))
        per_coil.append(field)
        total = add(total, field)
    return PointResult(True, (), tuple(per_coil), total)
