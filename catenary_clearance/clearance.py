"""Vertical clearance between a catenary and a piecewise-linear terrain."""

from __future__ import annotations

import math
from dataclasses import dataclass

from .solver import Catenary, CatenaryError


@dataclass(frozen=True)
class ClearanceResult:
    min_clearance: float
    x: float
    cable_y: float
    terrain_y: float
    collision: bool


def validate_terrain(points, x1, x2):
    if len(points) < 2:
        raise CatenaryError("terrain polyline needs at least two points")
    for (xa, _), (xb, _) in zip(points, points[1:]):
        if not xb > xa:
            raise CatenaryError("terrain x coordinates must be strictly increasing")
    if points[0][0] > x1 or points[-1][0] < x2:
        raise CatenaryError("terrain polyline must cover the whole span")


def _terrain_y_at(points, x):
    for (xa, ya), (xb, yb) in zip(points, points[1:]):
        if xa <= x <= xb:
            return ya + (yb - ya) * (x - xa) / (xb - xa)
    raise CatenaryError(f"x={x!r} outside terrain coverage")


def clearance_against_terrain(cat: Catenary, points) -> ClearanceResult:
    """Minimum vertical clearance over the span, computed analytically.

    Checks every terrain vertex and the stationary point inside each terrain
    segment where the cable slope equals the segment slope. Negative values
    are preserved and flagged as collisions.
    """
    validate_terrain(points, cat.x1, cat.x2)
    best_x = cat.x1
    best = cat.y(cat.x1) - _terrain_y_at(points, cat.x1)

    def consider(x):
        nonlocal best, best_x
        value = cat.y(x) - _terrain_y_at(points, x)
        if value < best:
            best, best_x = value, x

    for (xa, ya), (xb, yb) in zip(points, points[1:]):
        lo = max(xa, cat.x1)
        hi = min(xb, cat.x2)
        if lo > hi:
            continue
        consider(lo)
        consider(hi)
        slope = (yb - ya) / (xb - xa)
        xs = cat.x_vertex + cat.a * math.asinh(slope)
        if lo < xs < hi:
            consider(xs)
    return ClearanceResult(
        min_clearance=best,
        x=best_x,
        cable_y=cat.y(best_x),
        terrain_y=_terrain_y_at(points, best_x),
        collision=best < 0.0,
    )
