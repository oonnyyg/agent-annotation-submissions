"""Core numerics for the limb-darkened transit exposure model.

The star is a unit-radius disk with quadratic limb darkening
    I(mu) = 1 - u1 * (1 - mu) - u2 * (1 - mu) ** 2,
where mu = sqrt(1 - r**2) and r is the normalized radial coordinate.
The planet is an opaque disk of radius ratio ``radius_ratio`` moving on a
straight line at constant projected speed.
"""

from __future__ import annotations

import math
from dataclasses import dataclass


def _require_finite(name: str, value: float) -> float:
    value = float(value)
    if not math.isfinite(value):
        raise ValueError(f"{name} must be finite, got {value!r}")
    return value


@dataclass(frozen=True)
class LimbDarkening:
    """Quadratic limb-darkening law with u1, u2 >= 0 and u1 + u2 <= 1."""

    u1: float
    u2: float

    def __post_init__(self) -> None:
        u1 = _require_finite("u1", self.u1)
        u2 = _require_finite("u2", self.u2)
        if u1 < 0.0 or u2 < 0.0:
            raise ValueError("limb-darkening coefficients must be non-negative")
        if u1 + u2 > 1.0:
            raise ValueError("limb-darkening coefficients must sum to at most 1")
        object.__setattr__(self, "u1", u1)
        object.__setattr__(self, "u2", u2)

    def intensity(self, r: float) -> float:
        """Specific intensity at normalized radius ``r`` in [0, 1]."""
        mu = math.sqrt(max(0.0, 1.0 - r * r))
        one_minus_mu = 1.0 - mu
        return 1.0 - self.u1 * one_minus_mu - self.u2 * one_minus_mu**2


def total_stellar_flux(ld: LimbDarkening) -> float:
    """Integrated flux of the unocculted unit disk (analytic)."""
    return math.pi * (1.0 - ld.u1 / 3.0 - ld.u2 / 6.0)


class IntegrationError(RuntimeError):
    """Raised when adaptive quadrature fails to meet the requested tolerance."""


@dataclass(frozen=True)
class TransitGeometry:
    """Planet disk and its straight-line projected trajectory.

    ``radius_ratio`` in (0, 1], ``impact`` >= 0 (stellar radii),
    ``t_closest`` in hours, ``velocity`` > 0 (stellar radii per hour).
    """

    radius_ratio: float
    impact: float
    t_closest: float
    velocity: float

    def __post_init__(self) -> None:
        rp = _require_finite("radius_ratio", self.radius_ratio)
        b = _require_finite("impact", self.impact)
        t0 = _require_finite("t_closest", self.t_closest)
        v = _require_finite("velocity", self.velocity)
        if not 0.0 < rp <= 1.0:
            raise ValueError("radius_ratio must be in (0, 1]")
        if b < 0.0:
            raise ValueError("impact must be non-negative")
        if v <= 0.0:
            raise ValueError("velocity must be positive")
        object.__setattr__(self, "radius_ratio", rp)
        object.__setattr__(self, "impact", b)
        object.__setattr__(self, "t_closest", t0)
        object.__setattr__(self, "velocity", v)

    def separation(self, t: float) -> float:
        """Center-to-center separation in stellar radii at time ``t`` (hours)."""
        x = self.velocity * (t - self.t_closest)
        return math.hypot(x, self.impact)


def adaptive_simpson(func, a: float, b: float, tol: float, max_depth: int = 30) -> float:
    """Adaptive Simpson quadrature; raises IntegrationError if not converged."""
    if tol <= 0.0 or not math.isfinite(tol):
        raise ValueError("tolerance must be positive and finite")
    if a == b:
        return 0.0

    def simpson(fa, fm, fb, a, b):
        return (b - a) / 6.0 * (fa + 4.0 * fm + fb)

    fa, fb = func(a), func(b)
    m = 0.5 * (a + b)
    fm = func(m)
    whole = simpson(fa, fm, fb, a, b)

    def recurse(a, b, fa, fm, fb, whole, tol, depth):
        m = 0.5 * (a + b)
        lm, rm = 0.5 * (a + m), 0.5 * (m + b)
        flm, frm = func(lm), func(rm)
        left = simpson(fa, flm, fm, a, m)
        right = simpson(fm, frm, fb, m, b)
        delta = left + right - whole
        if depth <= 0:
            raise IntegrationError(
                f"adaptive Simpson did not converge on [{a}, {b}] "
                f"(|delta|={abs(delta):.3e}, tol={tol:.3e})"
            )
        if abs(delta) <= 15.0 * tol:
            return left + right + delta / 15.0
        return recurse(a, m, fa, flm, fm, left, tol, depth - 1) + recurse(
            m, b, fm, frm, fb, right, tol, depth - 1
        )

    return recurse(a, b, fa, fm, fb, whole, tol, max_depth)


def _blocked_flux(ld: LimbDarkening, rp: float, d: float, tol: float) -> float:
    """Stellar flux hidden by a planet disk of radius ``rp`` at separation ``d``.

    Reduces the 2-D overlap integral to a 1-D radial integral: for each
    radius r from the stellar center, the circle of radius r contributes its
    arc length 2 * phi(r) * r inside the planet disk, weighted by I(r).
    """
    if d >= 1.0 + rp:
        return 0.0

    def half_angle(r: float) -> float:
        if d == 0.0:
            return math.pi  # concentric: any r <= rp is fully covered
        if r + d <= rp:
            return math.pi
        cos_phi = (r * r + d * d - rp * rp) / (2.0 * r * d)
        return math.acos(max(-1.0, min(1.0, cos_phi)))

    def integrand(r: float) -> float:
        return 2.0 * half_angle(r) * r * ld.intensity(r)

    r_lo = max(0.0, d - rp)
    r_hi = min(1.0, d + rp)
    # Split at kinks: full-inclusion boundary and the stellar limb.
    cuts = [r_lo]
    for c in (rp - d, 1.0):
        if r_lo < c < r_hi:
            cuts.append(c)
    cuts.append(r_hi)

    total = 0.0
    span = r_hi - r_lo
    for a, b in zip(cuts, cuts[1:]):
        if b > a:
            total += adaptive_simpson(integrand, a, b, tol * (b - a) / span)
    return total


def instantaneous_flux(
    ld: LimbDarkening, geom: TransitGeometry, t: float, tol: float = 1e-10
) -> float:
    """Normalized stellar flux at time ``t`` (hours); 1.0 when unocculted."""
    _require_finite("t", t)
    blocked = _blocked_flux(ld, geom.radius_ratio, geom.separation(t), tol)
    return 1.0 - blocked / total_stellar_flux(ld)


def _contact_times(geom: TransitGeometry, c: float) -> list[float]:
    """Times when the center separation equals ``c`` (empty if never)."""
    if c <= geom.impact:
        return []
    dt = math.sqrt(c * c - geom.impact * geom.impact) / geom.velocity
    return [geom.t_closest - dt, geom.t_closest + dt]


def external_contact_interval(geom: TransitGeometry) -> tuple[float, float] | None:
    """First/last external contact (separation = 1 + radius_ratio).

    Returns None when the planet never overlaps the star, including the
    purely tangent case (zero-duration interval is reported as empty).
    """
    times = _contact_times(geom, 1.0 + geom.radius_ratio)
    if not times:
        return None
    return times[0], times[1]


def exposure_averaged_flux(
    ld: LimbDarkening,
    geom: TransitGeometry,
    t_center: float,
    duration: float,
    tol: float = 1e-8,
) -> float:
    """Mean normalized flux over [t_center - d/2, t_center + d/2].

    Zero duration returns the instantaneous value. The interval is split at
    contact times so the adaptive quadrature never straddles a kink.
    """
    _require_finite("t_center", t_center)
    _require_finite("duration", duration)
    if duration < 0.0:
        raise ValueError("duration must be non-negative")
    if duration == 0.0:
        return instantaneous_flux(ld, geom, t_center, tol)

    start = t_center - 0.5 * duration
    end = t_center + 0.5 * duration
    splits = [start]
    for c in (1.0 + geom.radius_ratio, abs(1.0 - geom.radius_ratio)):
        for tc in _contact_times(geom, c):
            if start < tc < end:
                splits.append(tc)
    splits.append(end)
    splits.sort()

    flux = total_stellar_flux(ld)

    def lightcurve(t: float) -> float:
        blocked = _blocked_flux(ld, geom.radius_ratio, geom.separation(t), tol * 0.1)
        return 1.0 - blocked / flux

    integral = 0.0
    for a, b in zip(splits, splits[1:]):
        if b > a:
            integral += adaptive_simpson(lightcurve, a, b, tol * (b - a) / duration)
    return integral / duration
