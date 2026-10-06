"""Minimum vertical clearance between the catenary and a terrain polyline.

The terrain is piecewise linear. On each segment the gap
g(x) = y_cable(x) - y_terrain(x) has derivative sinh((x-xv)/a) - m
(m = segment slope), so the only interior stationary point is
x* = xv + a*asinh(m). Checking both segment endpoints and x* (when it
lies inside) gives the exact minimum, independent of any sampling grid.
"""

import math


def _terrain_y(pts, x):
    for i in range(len(pts) - 1):
        xa, ya = pts[i]
        xb, yb = pts[i + 1]
        if xa <= x <= xb:
            t = (x - xa) / (xb - xa)
            return ya + t * (yb - ya)
    return pts[-1][1] if x > pts[-1][0] else pts[0][1]


def min_clearance(sol, terrain):
    """Return dict with min vertical clearance, its x, and collision flag."""
    x0, x1 = sol.x0, sol.x1
    best = (math.inf, None)

    def consider(x):
        nonlocal best
        gap = sol.y(x) - _terrain_y(terrain, x)
        if gap < best[0]:
            best = (gap, x)

    for i in range(len(terrain) - 1):
        xa, ya = terrain[i]
        xb, yb = terrain[i + 1]
        lo = max(xa, x0)
        hi = min(xb, x1)
        if lo > hi:
            continue
        m = (yb - ya) / (xb - xa)
        consider(lo)
        consider(hi)
        xs = sol.xv + sol.a * math.asinh(m)
        if lo < xs < hi:
            consider(xs)

    gap, x = best
    return {
        "min_clearance": gap,
        "x": x,
        "cable_y": sol.y(x),
        "terrain_y": _terrain_y(terrain, x),
        "collision": gap < 0.0,
    }


def terrain_y(pts, x):
    return _terrain_y(pts, x)
