"""2-D polygon geometry helpers (standard library only).

Body coordinates: x horizontal to the right, y vertical up.
Polygons may be concave and use either winding, but must be simple
(non self-intersecting) and have non-zero area.
"""

from __future__ import annotations

import math

EPS = 1e-12


def polygon_signed_area(vertices):
    area = 0.0
    n = len(vertices)
    for i in range(n):
        x1, y1 = vertices[i]
        x2, y2 = vertices[(i + 1) % n]
        area += x1 * y2 - x2 * y1
    return 0.5 * area


def polygon_area(vertices):
    return abs(polygon_signed_area(vertices))


def polygon_centroid(vertices):
    """Centroid of a simple polygon (works for either winding)."""
    a = 0.0
    cx = 0.0
    cy = 0.0
    n = len(vertices)
    for i in range(n):
        x1, y1 = vertices[i]
        x2, y2 = vertices[(i + 1) % n]
        cross = x1 * y2 - x2 * y1
        a += cross
        cx += (x1 + x2) * cross
        cy += (y1 + y2) * cross
    a *= 0.5
    if abs(a) < EPS:
        raise ValueError("polygon area is zero; centroid undefined")
    return (cx / (6.0 * a), cy / (6.0 * a))


def _orientation(a, b, c):
    return (b[0] - a[0]) * (c[1] - a[1]) - (b[1] - a[1]) * (c[0] - a[0])


def _segments_properly_intersect(p1, p2, p3, p4):
    """True if open segments p1p2 and p3p4 cross (proper intersection)."""
    d1 = _orientation(p3, p4, p1)
    d2 = _orientation(p3, p4, p2)
    d3 = _orientation(p1, p2, p3)
    d4 = _orientation(p1, p2, p4)
    return (d1 * d2 < 0.0) and (d3 * d4 < 0.0)


def validate_simple_polygon(vertices):
    """Reject degenerate or self-intersecting polygons. Returns None or raises."""
    n = len(vertices)
    if n < 3:
        raise ValueError("polygon needs at least 3 vertices")
    for v in vertices:
        if len(v) != 2 or not all(math.isfinite(c) for c in v):
            raise ValueError("vertices must be finite [x, y] pairs")
    if polygon_area(vertices) < EPS:
        raise ValueError("polygon area is zero")
    # Non-adjacent edge pairs must not properly intersect.
    edges = [(vertices[i], vertices[(i + 1) % n]) for i in range(n)]
    for i in range(n):
        for j in range(i + 1, n):
            # Skip edges sharing a vertex (adjacent, incl. wrap-around).
            if j == i or j == (i + 1) % n or i == (j + 1) % n:
                continue
            if _segments_properly_intersect(edges[i][0], edges[i][1],
                                            edges[j][0], edges[j][1]):
                raise ValueError(
                    f"polygon self-intersects between edge {i} and edge {j}")


def rotate_polygon(vertices, angle_rad):
    c = math.cos(angle_rad)
    s = math.sin(angle_rad)
    return [(c * x - s * y, s * x + c * y) for x, y in vertices]


def rotate_point(point, angle_rad):
    c = math.cos(angle_rad)
    s = math.sin(angle_rad)
    x, y = point
    return (c * x - s * y, s * x + c * y)


def translate_polygon(vertices, dy):
    return [(x, y + dy) for x, y in vertices]


def clip_below_waterline(vertices, waterline=0.0):
    """Sutherland-Hodgman clip of a simple polygon against y <= waterline.

    Returns the (possibly empty) list of clipped sub-polygons. For a simple
    input polygon clipped by a half-plane the result is a single simple
    polygon (the half-plane intersection of a simple polygon is connected
    only if the polygon is x-monotone... in general it can split), so we
    split the clipped vertex loop into disjoint loops at waterline cuts.
    """
    n = len(vertices)
    out = []
    for i in range(n):
        cur = vertices[i]
        nxt = vertices[(i + 1) % n]
        cur_in = cur[1] <= waterline + EPS
        nxt_in = nxt[1] <= waterline + EPS
        if cur_in:
            out.append(cur)
        if cur_in != nxt_in:
            t = (waterline - cur[1]) / (nxt[1] - cur[1])
            out.append((cur[0] + t * (nxt[0] - cur[0]), waterline))
    if len(out) < 3:
        return []
    if polygon_area(out) < EPS:
        return []
    return [out]
