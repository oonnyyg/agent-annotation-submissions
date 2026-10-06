"""Core geometry and K/L estimation for a rectangular window.

The window is the half-open rectangle [0, width) x [0, height) with the
origin at the lower-left corner.  No toroidal wrapping is applied.
"""

from __future__ import annotations

import math
from dataclasses import dataclass


@dataclass(frozen=True)
class Window:
    width: float
    height: float

    def __post_init__(self) -> None:
        if not (self.width > 0 and self.height > 0):
            raise ValueError("window width and height must be positive")

    @property
    def area(self) -> float:
        return self.width * self.height

    @property
    def max_radius(self) -> float:
        return min(self.width, self.height) / 2.0


def validate_points(points, window: Window):
    pts = [(float(x), float(y)) for x, y in points]
    if len(pts) < 2:
        raise ValueError("at least two points are required")
    for x, y in pts:
        if not (0.0 <= x < window.width and 0.0 <= y < window.height):
            raise ValueError(f"point ({x}, {y}) lies outside the window")
    if len(set(pts)) != len(pts):
        raise ValueError("duplicate point locations are not allowed")
    return pts


def validate_radii(radii, window: Window):
    rs = [float(r) for r in radii]
    if not rs:
        raise ValueError("radius grid must not be empty")
    if rs[0] != 0.0:
        raise ValueError("radius grid must start at 0")
    if any(b <= a for a, b in zip(rs, rs[1:])):
        raise ValueError("radius grid must be strictly increasing")
    if rs[-1] > window.max_radius:
        raise ValueError(
            "radius grid must not exceed half the shorter side "
            f"({window.max_radius})"
        )
    return rs


def ordered_pair_distances(points):
    """Distances and absolute displacements for ordered pairs i != j."""
    pairs = []
    n = len(points)
    for i in range(n):
        xi, yi = points[i]
        for j in range(n):
            if i == j:
                continue
            xj, yj = points[j]
            dx = xi - xj
            dy = yi - yj
            pairs.append((math.hypot(dx, dy), abs(dx), abs(dy)))
    return pairs


def translation_weight(window: Window, dx: float, dy: float) -> float:
    """Edge-correction weight for a pair with displacement (dx, dy).

    Weight is the window area divided by the area of the overlap between
    the window and its translate by (dx, dy).
    """
    overlap = (window.width - dx) * (window.height - dy)
    if overlap <= 0.0:
        return 0.0
    return window.area / overlap


def k_estimate(window: Window, pairs, radii, correct: bool):
    """Cumulative ordered pair counts and K values over the radius grid.

    Points at distance exactly equal to a radius are included.
    """
    n_ordered = len(pairs)
    counts = []
    k_values = []
    for r in radii:
        total = 0.0
        count = 0
        for dist, dx, dy in pairs:
            if dist <= r:
                count += 1
                if correct:
                    total += translation_weight(window, dx, dy)
        if correct:
            k = window.area * total / n_ordered
        else:
            k = window.area * count / n_ordered
        counts.append(count)
        k_values.append(k)
    return counts, k_values


def l_from_k(k_values):
    return [math.sqrt(k / math.pi) for k in k_values]
