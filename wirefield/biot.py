"""Biot-Savart field of finite straight current segments."""

from __future__ import annotations

from .vector import Vec3, cross, dot, norm, sub

MU0_OVER_4PI = 1e-7  # mu0 / (4*pi) with mu0 = 4*pi*1e-7 H/m


def segment_field(point: Vec3, a: Vec3, b: Vec3, current: float) -> Vec3:
    """Field at `point` of a straight segment a->b carrying `current` (A).

    Uses the closed-form finite-segment Biot-Savart result
        B = mu0*I/(4*pi) * (r1 x r2) * (|r1|+|r2|)
            / (|r1|*|r2| * (|r1|*|r2| + r1.r2))
    where r1, r2 point from the field point to the segment endpoints.
    A point collinear with the segment (including its extension line)
    gives r1 x r2 = 0, hence zero field. The caller must ensure the
    point is not on the segment itself (denominator would vanish).
    """
    r1 = sub(a, point)
    r2 = sub(b, point)
    n1 = norm(r1)
    n2 = norm(r2)
    cx = cross(r1, r2)
    denom = n1 * n2 * (n1 * n2 + dot(r1, r2))
    factor = MU0_OVER_4PI * current * (n1 + n2) / denom
    return (cx[0] * factor, cx[1] * factor, cx[2] * factor)


def point_segment_distance(point: Vec3, a: Vec3, b: Vec3) -> float:
    """Shortest distance from `point` to the finite segment a-b."""
    ab = sub(b, a)
    ap = sub(point, a)
    length_sq = dot(ab, ab)
    t = dot(ap, ab) / length_sq
    t = max(0.0, min(1.0, t))
    closest = (a[0] + ab[0] * t, a[1] + ab[1] * t, a[2] + ab[2] * t)
    return norm(sub(point, closest))
