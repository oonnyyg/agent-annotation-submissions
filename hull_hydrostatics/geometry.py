"""Polygon geometry: validation, rotation, waterline clipping, area/centroid.

Body coordinates: x horizontal right, y vertical up. Positive heel angle is
counterclockwise. The waterline is the horizontal line y = 0 in world
coordinates; the submerged part of a polygon is the part with y <= 0.
"""

from __future__ import annotations

import math

_EPS = 1e-12


class GeometryError(ValueError):
    """Raised when a polygon fails geometric validation."""


def signed_area(points):
    """Signed area via the shoelace formula (positive for CCW order)."""
    total = 0.0
    n = len(points)
    for i in range(n):
        x1, y1 = points[i]
        x2, y2 = points[(i + 1) % n]
        total += x1 * y2 - x2 * y1
    return 0.5 * total


def _segments_intersect(p1, p2, p3, p4):
    """True if closed segments p1p2 and p3p4 share any point."""

    def cross(o, a, b):
        return (a[0] - o[0]) * (b[1] - o[1]) - (a[1] - o[1]) * (b[0] - o[0])

    def on_segment(o, a, b):
        return (
            min(a[0], b[0]) - _EPS <= o[0] <= max(a[0], b[0]) + _EPS
            and min(a[1], b[1]) - _EPS <= o[1] <= max(a[1], b[1]) + _EPS
        )

    d1 = cross(p3, p4, p1)
    d2 = cross(p3, p4, p2)
    d3 = cross(p1, p2, p3)
    d4 = cross(p1, p2, p4)
    if ((d1 > _EPS and d2 < -_EPS) or (d1 < -_EPS and d2 > _EPS)) and (
        (d3 > _EPS and d4 < -_EPS) or (d3 < -_EPS and d4 > _EPS)
    ):
        return True
    if abs(d1) <= _EPS and on_segment(p1, p3, p4):
        return True
    if abs(d2) <= _EPS and on_segment(p2, p3, p4):
        return True
    if abs(d3) <= _EPS and on_segment(p3, p1, p2):
        return True
    if abs(d4) <= _EPS and on_segment(p4, p1, p2):
        return True
    return False


def validate_polygon(points):
    """Validate a simple (non-self-intersecting) closed polygon.

    Accepts either winding order and concave shapes. Returns a list of
    (x, y) tuples in counterclockwise order. Raises GeometryError for
    degenerate or self-intersecting input.
    """
    if not isinstance(points, (list, tuple)) or len(points) < 3:
        raise GeometryError("polygon needs at least 3 vertices")
    pts = []
    for v in points:
        if (
            not isinstance(v, (list, tuple))
            or len(v) != 2
            or not all(isinstance(c, (int, float)) and math.isfinite(c) for c in v)
        ):
            raise GeometryError(f"invalid vertex: {v!r}")
        pts.append((float(v[0]), float(v[1])))

    # Remove duplicate consecutive vertices (incl. closing duplicate).
    cleaned = []
    for p in pts:
        if (
            not cleaned
            or abs(p[0] - cleaned[-1][0]) > _EPS
            or abs(p[1] - cleaned[-1][1]) > _EPS
        ):
            cleaned.append(p)
    if len(cleaned) > 1 and (
        abs(cleaned[0][0] - cleaned[-1][0]) <= _EPS
        and abs(cleaned[0][1] - cleaned[-1][1]) <= _EPS
    ):
        cleaned.pop()
    if len(cleaned) < 3:
        raise GeometryError("polygon degenerates to fewer than 3 distinct vertices")
    pts = cleaned
    n = len(pts)

    # Reject zero-length edges and 180-degree (collinear) spikes.
    for i in range(n):
        a = pts[i]
        b = pts[(i + 1) % n]
        c = pts[(i + 2) % n]
        ex, ey = b[0] - a[0], b[1] - a[1]
        if ex * ex + ey * ey <= _EPS * _EPS:
            raise GeometryError("zero-length edge")
        fx, fy = c[0] - b[0], c[1] - b[1]
        cross = ex * fy - ey * fx
        dot = ex * fx + ey * fy
        if abs(cross) <= _EPS and dot < 0:
            raise GeometryError("polygon has a degenerate spike vertex")

    area = signed_area(pts)
    if abs(area) <= _EPS:
        raise GeometryError("polygon has zero area")

    # Self-intersection: compare non-adjacent edge pairs.
    edges = [(pts[i], pts[(i + 1) % n]) for i in range(n)]
    for i in range(n):
        for j in range(i + 1, n):
            if j == i or (j == (i + 1) % n) or (i == (j + 1) % n):
                continue
            if i == 0 and j == n - 1:
                continue  # adjacent through the closing edge
            if _segments_intersect(edges[i][0], edges[i][1], edges[j][0], edges[j][1]):
                raise GeometryError("polygon is self-intersecting")

    if area < 0:
        pts = list(reversed(pts))
    return pts


def rotate_points(points, angle_rad):
    """Rotate points counterclockwise about the origin."""
    c = math.cos(angle_rad)
    s = math.sin(angle_rad)
    return [(x * c - y * s, x * s + y * c) for x, y in points]


def translate_points(points, dx, dy):
    return [(x + dx, y + dy) for x, y in points]


def clip_below_waterline(points):
    """Intersect the polygon with the half-plane y <= 0 (Sutherland-Hodgman).

    Returns the submerged polygon as a list of vertices (may be empty).
    """
    if not points:
        return []
    result = []
    n = len(points)
    for i in range(n):
        cur = points[i]
        nxt = points[(i + 1) % n]
        cur_in = cur[1] <= 0.0
        nxt_in = nxt[1] <= 0.0
        if cur_in:
            result.append(cur)
        if cur_in != nxt_in:
            t = cur[1] / (cur[1] - nxt[1])
            ix = cur[0] + t * (nxt[0] - cur[0])
            result.append((ix, 0.0))
    return result


def area_centroid(points):
    """Area and centroid of a polygon. Returns (area, (cx, cy)).

    Assumes a simple polygon in either winding order; area is returned
    positive. Returns zero area and origin centroid for empty input.
    """
    if len(points) < 3:
        return 0.0, (0.0, 0.0)
    a = 0.0
    cx = 0.0
    cy = 0.0
    n = len(points)
    for i in range(n):
        x1, y1 = points[i]
        x2, y2 = points[(i + 1) % n]
        cross = x1 * y2 - x2 * y1
        a += cross
        cx += (x1 + x2) * cross
        cy += (y1 + y2) * cross
    a *= 0.5
    if abs(a) <= _EPS:
        return 0.0, (0.0, 0.0)
    cx /= 6.0 * a
    cy /= 6.0 * a
    return abs(a), (cx, cy)
