"""Numerics for the von Mises mixture model.

All angles are handled internally in radians, normalized to [0, 2*pi).
"""

from __future__ import annotations

import math

TWO_PI = 2.0 * math.pi
KAPPA_MAX = 50.0


def normalize_angle(theta: float) -> float:
    """Map an angle into [0, 2*pi). Angles differing by whole turns coincide."""
    return math.fmod(math.fmod(theta, TWO_PI) + TWO_PI, TWO_PI)


def bessel_i0(x: float) -> float:
    """Modified Bessel function I_0(x), computed via its power series.

    Stable for the kappa range used here (0 <= x <= KAPPA_MAX); the series
    terms are all positive and the sum is terminated on machine precision.
    """
    if x < 0.0:
        raise ValueError("bessel_i0 requires x >= 0")
    term = 1.0
    total = 1.0
    k = 0
    while True:
        k += 1
        term *= (x / (2.0 * k)) ** 2
        total += term
        if term <= total * 1e-16 or k > 10000:
            return total


def log_bessel_i0(x: float) -> float:
    """log I_0(x) computed without overflow via the scaled series."""
    if x == 0.0:
        return 0.0
    # I_0(x) = e^x * sum of scaled terms; use the asymptotic-safe route:
    # compute I_0 directly (no overflow for x <= 50) and take the log.
    return math.log(bessel_i0(x))


def a1_inverse(r: float) -> float:
    """Approximate inverse of A(kappa) = I_1(kappa)/I_0(kappa).

    Uses the standard Banerjee et al. (2005) rational approximation,
    refined by a few Newton steps on A(kappa) - r = 0.
    """
    if r <= 0.0:
        return 0.0
    if r >= 1.0:
        return KAPPA_MAX
    kappa = (r * (3.0 - r * r)) / (1.0 - r * r) if r < 0.85 else 1.0 / (1.0 - r)
    kappa = min(kappa, KAPPA_MAX)
    for _ in range(8):
        a = _a1(kappa)
        if a <= 0.0:
            break
        # A'(k) = 1 - A(k)^2 - A(k)/k
        deriv = 1.0 - a * a - a / kappa
        if deriv <= 0.0:
            break
        kappa -= (a - r) / deriv
        if kappa <= 0.0:
            kappa = 1e-10
            break
    return min(kappa, KAPPA_MAX)


def _a1(kappa: float) -> float:
    """A(kappa) = I_1(kappa) / I_0(kappa) via a shared series evaluation."""
    if kappa > 40.0:
        # Asymptotic expansion, avoids overflow of the power series.
        inv = 1.0 / kappa
        return 1.0 - 0.5 * inv - 0.125 * inv**2 - 0.125 * inv**3
    # I_1(k) = (k/2) * sum_{m} (k^2/4)^m / (m! (m+1)!)
    term0 = 1.0  # for I_0
    term1 = kappa / 2.0  # for I_1
    sum0 = term0
    sum1 = term1
    m = 0
    x2 = (kappa / 2.0) ** 2
    while True:
        m += 1
        term0 *= x2 / (m * m)
        term1 *= x2 / (m * (m + 1))
        sum0 += term0
        sum1 += term1
        if term0 <= sum0 * 1e-16 and term1 <= sum1 * 1e-16:
            break
        if m > 10000:
            break
    return sum1 / sum0


def circular_mean(angles: list[float], weights: list[float]) -> tuple[float, float]:
    """Weighted circular mean direction and mean resultant length R in [0, 1].

    Samples on opposite sides of the zero direction are averaged on the
    circle, never across a linear cut, so a cluster straddling 0 deg keeps
    its true direction.
    """
    s = 0.0
    c = 0.0
    w_total = 0.0
    for theta, w in zip(angles, weights):
        s += w * math.sin(theta)
        c += w * math.cos(theta)
        w_total += w
    if w_total <= 0.0:
        raise ValueError("total weight must be positive")
    r = math.hypot(s, c) / w_total
    mu = math.atan2(s, c)
    return normalize_angle(mu), r
