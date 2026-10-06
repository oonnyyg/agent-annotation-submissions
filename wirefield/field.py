"""Biot-Savart field of finite straight segments and superposition."""

from __future__ import annotations

import math

from .coil import Coil
from .vector import Vec3, add, cross, dot, norm, scale, sub

MU0 = 4e-7 * math.pi
MU0_OVER_4PI = MU0 / (4.0 * math.pi)

ZERO: Vec3 = (0.0, 0.0, 0.0)


def segment_field(point: Vec3, a: Vec3, b: Vec3, current: float) -> Vec3:
    """Field (tesla) at `point` of a straight segment a->b carrying `current`.

    Exact closed form for a finite straight filament:
    B = mu0*I/(4*pi*h^2) * (r1.u/|r1| - r2.u/|r2|) * (u x d)
    where u is the unit tangent, r1 = p-a, r2 = p-b, and d is the
    perpendicular offset of the point from the segment line (h = |d|).

    A point exactly on the segment line (h == 0), including points on the
    extension of the segment beyond its endpoints, contributes zero: the
    cross product vanishes there. It is NOT treated as an infinite wire.
    """
    seg = sub(b, a)
    seg_len = norm(seg)
    if seg_len == 0.0:
        raise ValueError("zero-length segment")
    u = scale(seg, 1.0 / seg_len)
    r1 = sub(point, a)
    r2 = sub(point, b)
    d = sub(r1, scale(u, dot(r1, u)))
    h2 = dot(d, d)
    if h2 == 0.0:
        return ZERO
    n1 = norm(r1)
    n2 = norm(r2)
    factor = dot(r1, u) / n1 - dot(r2, u) / n2
    direction = cross(u, d)
    coeff = MU0_OVER_4PI * current * factor / h2
    return scale(direction, coeff)


def point_segment_distance(point: Vec3, a: Vec3, b: Vec3) -> float:
    """Shortest distance from `point` to the finite segment a-b."""
    seg = sub(b, a)
    length2 = dot(seg, seg)
    if length2 == 0.0:
        raise ValueError("zero-length segment")
    t = dot(sub(point, a), seg) / length2
    t = max(0.0, min(1.0, t))
    closest = add(a, scale(seg, t))
    return norm(sub(point, closest))


def coil_field(coil: Coil, point: Vec3, current: float | None = None) -> Vec3:
    """Total field of one closed coil at `point`."""
    if current is None:
        current = coil.current
    total = ZERO
    for a, b in coil.segments():
        total = add(total, segment_field(point, a, b, current))
    return total


def coil_min_distance(coil: Coil, point: Vec3) -> float:
    """Minimum distance from `point` to any segment of the coil."""
    return min(point_segment_distance(point, a, b) for a, b in coil.segments())


def field_per_coil(
    coils: list[Coil], point: Vec3, currents: dict[str, float] | None = None
) -> dict[str, Vec3]:
    """Field contribution of each coil, keyed by coil id."""
    result: dict[str, Vec3] = {}
    for coil in coils:
        current = coil.current if currents is None else currents[coil.coil_id]
        result[coil.coil_id] = coil_field(coil, point, current)
    return result


def total_field(contributions: dict[str, Vec3]) -> Vec3:
    total = ZERO
    for vec in contributions.values():
        total = add(total, vec)
    return total
