"""Numerically stable von Mises building blocks."""

from __future__ import annotations

import math

TWO_PI = 2.0 * math.pi
KAPPA_MAX = 50.0


def norm_angle(theta: float) -> float:
    """Normalize an angle to [0, 2*pi). Angles differing by full turns coincide."""
    return theta % TWO_PI


def bessel_i0(x: float) -> float:
    """Modified Bessel function I0 via its power series (accurate for |x| <= 50)."""
    if x == 0.0:
        return 1.0
    term = 1.0
    total = 1.0
    y = (x * x) / 4.0
    k = 1
    while True:
        term *= y / (k * k)
        total += term
        if term <= total * 1e-16:
            return total
        k += 1


def bessel_i1(x: float) -> float:
    """Modified Bessel function I1 via its power series."""
    term = x / 2.0
    total = term
    y = (x * x) / 4.0
    k = 1
    while True:
        term *= y / (k * (k + 1))
        total += term
        if term <= total * 1e-16:
            return total
        k += 1


def a1(kappa: float) -> float:
    """Ratio I1(kappa)/I0(kappa) from the series values (stable for kappa <= 50)."""
    if kappa == 0.0:
        return 0.0
    return bessel_i1(kappa) / bessel_i0(kappa)


def log_norm_const(kappa: float) -> float:
    """Log of the von Mises normalizing constant 2*pi*I0(kappa)."""
    return math.log(TWO_PI) + math.log(bessel_i0(kappa))


def log_density(theta: float, mu: float, kappa: float) -> float:
    """Log von Mises density; kappa == 0 is the uniform distribution."""
    if kappa == 0.0:
        return -math.log(TWO_PI)
    return kappa * math.cos(theta - mu) - log_norm_const(kappa)


def circular_mean(angles: list[float], weights: list[float]) -> tuple[float, float]:
    """Weighted circular mean. Returns (mean direction, mean resultant length R).

    Uses vector sums, so samples straddling 0/2*pi average correctly.
    R == 0 means no preferred direction (e.g. perfectly uniform data).
    """
    s = 0.0
    c = 0.0
    wsum = 0.0
    for theta, w in zip(angles, weights):
        s += w * math.sin(theta)
        c += w * math.cos(theta)
        wsum += w
    if wsum <= 0.0:
        raise ValueError("total weight must be positive")
    r = math.hypot(s, c) / wsum
    mu = math.atan2(s, c) % TWO_PI if r > 0.0 else 0.0
    return mu, r


def kappa_from_r(r: float) -> float:
    """Approximate kappa from mean resultant length R (Banerjee et al. 2005)."""
    if r <= 0.0:
        return 0.0
    if r >= 1.0:
        return KAPPA_MAX
    r2 = r * r
    kappa = r * (2.0 - r2) / (1.0 - r2)
    return min(kappa, KAPPA_MAX)
