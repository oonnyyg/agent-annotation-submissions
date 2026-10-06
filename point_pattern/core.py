"""Core computations: validation, translation-corrected K/L, Monte Carlo test."""

from __future__ import annotations

import math
import random
from dataclasses import dataclass


class PatternError(ValueError):
    """Raised for invalid study regions, point patterns, or radius grids."""


@dataclass(frozen=True)
class PointPattern:
    """Points in a half-open rectangle [0, width) x [0, height), origin lower-left."""

    width: float
    height: float
    points: tuple[tuple[float, float], ...]

    def __post_init__(self) -> None:
        if not (math.isfinite(self.width) and self.width > 0):
            raise PatternError("width must be a positive finite number")
        if not (math.isfinite(self.height) and self.height > 0):
            raise PatternError("height must be a positive finite number")
        if len(self.points) < 2:
            raise PatternError("at least two distinct points are required")
        seen: set[tuple[float, float]] = set()
        for x, y in self.points:
            if not (math.isfinite(x) and math.isfinite(y)):
                raise PatternError(f"non-finite coordinate: ({x}, {y})")
            if not (0.0 <= x < self.width and 0.0 <= y < self.height):
                raise PatternError(
                    f"point ({x}, {y}) outside half-open region "
                    f"[0, {self.width}) x [0, {self.height})"
                )
            if (x, y) in seen:
                raise PatternError(f"duplicate point location: ({x}, {y})")
            seen.add((x, y))

    @property
    def n(self) -> int:
        return len(self.points)

    @property
    def area(self) -> float:
        return self.width * self.height


@dataclass(frozen=True)
class RadiusGrid:
    """Strictly increasing radii from 0 up to half the shorter side (inclusive)."""

    radii: tuple[float, ...]
    max_radius: float

    def __post_init__(self) -> None:
        if len(self.radii) < 2:
            raise PatternError("radius grid needs at least two values")
        if self.radii[0] != 0.0:
            raise PatternError("radius grid must start at 0")
        for prev, cur in zip(self.radii, self.radii[1:]):
            if not math.isfinite(cur) or cur <= prev:
                raise PatternError("radius grid must be finite and strictly increasing")
        if self.radii[-1] > self.max_radius + 1e-12:
            raise PatternError(
                f"largest radius {self.radii[-1]} exceeds half the shorter side "
                f"({self.max_radius})"
            )

    @classmethod
    def uniform(cls, width: float, height: float, count: int) -> "RadiusGrid":
        max_radius = min(width, height) / 2.0
        if not isinstance(count, int) or count < 2:
            raise PatternError("grid count must be an integer >= 2")
        step = max_radius / (count - 1)
        radii = tuple(i * step for i in range(count - 1)) + (max_radius,)
        return cls(radii=radii, max_radius=max_radius)


def _ordered_pair_stats(
    width: float, height: float, points: tuple[tuple[float, float], ...]
) -> list[tuple[float, float]]:
    """(distance, translation weight) for every ordered pair of distinct points.

    Weight = window area / overlap of the window with its translate by the
    pair displacement (translation edge correction). The window is NOT
    treated as periodic.
    """
    area = width * height
    stats: list[tuple[float, float]] = []
    n = len(points)
    for i in range(n):
        xi, yi = points[i]
        for j in range(n):
            if i == j:
                continue
            dx = xi - points[j][0]
            dy = yi - points[j][1]
            overlap = (width - abs(dx)) * (height - abs(dy))
            dist = math.hypot(dx, dy)
            stats.append((dist, area / overlap))
    return stats


def compute_k(
    width: float,
    height: float,
    points: tuple[tuple[float, float], ...],
    radii: tuple[float, ...],
) -> dict[str, list[float]]:
    """Cumulative ordered-pair counts, uncorrected K, and corrected K per radius.

    K(r) = area / (n * (n-1)) * sum of weights over ordered pairs with
    distance <= r (pairs exactly at r are included). The uncorrected
    variant uses weight 1 for every pair.
    """
    n = len(points)
    area = width * height
    stats = _ordered_pair_stats(width, height, points)
    counts: list[float] = []
    k_uncorrected: list[float] = []
    k_corrected: list[float] = []
    for r in radii:
        count = 0
        weight_sum = 0.0
        for dist, weight in stats:
            if dist <= r:
                count += 1
                weight_sum += weight
        counts.append(float(count))
        k_uncorrected.append(area * count / (n * (n - 1)))
        k_corrected.append(area * weight_sum / (n * (n - 1)))
    return {
        "count": counts,
        "k_uncorrected": k_uncorrected,
        "k_corrected": k_corrected,
    }


def compute_l(k_values: list[float]) -> list[float]:
    """L(r) = sqrt(K(r) / pi), elementwise."""
    return [math.sqrt(max(k, 0.0) / math.pi) for k in k_values]


def _l_minus_r_statistic(l_values: list[float], radii: tuple[float, ...]) -> float:
    return max(abs(l - r) for l, r in zip(l_values, radii))


def simulate_envelope(
    width: float,
    height: float,
    n_points: int,
    radii: tuple[float, ...],
    n_simulations: int,
    seed: int,
    observed_l: list[float],
) -> dict:
    """Simulate CSR configurations and run a Monte Carlo test.

    Each simulation draws n_points i.i.d. uniform locations in the same
    rectangle. The test statistic is max over the radius grid of
    |L(r) - r|. The reported p-value is the plus-one corrected upper-tail
    Monte Carlo probability, with ties counted toward the tail. Also
    returns pointwise min/max envelopes of simulated L curves (NOT a
    simultaneous confidence band).
    """
    if not isinstance(n_simulations, int) or n_simulations < 1:
        raise PatternError("number of simulations must be a positive integer")
    rng = random.Random(seed)
    observed_stat = _l_minus_r_statistic(observed_l, radii)
    envelope_min = [math.inf] * len(radii)
    envelope_max = [-math.inf] * len(radii)
    exceedances = 0
    for _ in range(n_simulations):
        sim_points = tuple(
            (rng.random() * width, rng.random() * height) for _ in range(n_points)
        )
        sim_k = compute_k(width, height, sim_points, radii)["k_corrected"]
        sim_l = compute_l(sim_k)
        for idx, value in enumerate(sim_l):
            if value < envelope_min[idx]:
                envelope_min[idx] = value
            if value > envelope_max[idx]:
                envelope_max[idx] = value
        if _l_minus_r_statistic(sim_l, radii) >= observed_stat:
            exceedances += 1
    p_value = (exceedances + 1) / (n_simulations + 1)
    return {
        "statistic": observed_stat,
        "p_value": p_value,
        "exceedances": exceedances,
        "n_simulations": n_simulations,
        "seed": seed,
        "envelope_min": envelope_min,
        "envelope_max": envelope_max,
    }
