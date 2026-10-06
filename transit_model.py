"""Occultation of a quadratic limb-darkened star by an opaque planet.

Only the Python standard library is used.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field


@dataclass
class QuadratureResult:
    value: float
    converged: bool
    evaluations: int


def adaptive_simpson(func, a, b, tol, max_depth=30):
    """Adaptive Simpson integration of ``func`` over [a, b].

    Never silently clamps the value; non-convergence is reported.
    """
    if b <= a:
        return QuadratureResult(0.0, True, 0)

    evaluations = 0

    def f(x):
        nonlocal evaluations
        evaluations += 1
        return func(x)

    fa, fm, fb = f(a), f(0.5 * (a + b)), f(b)
    whole = (b - a) * (fa + 4.0 * fm + fb) / 6.0

    def recurse(a, b, fa, fm, fb, whole, tol, depth):
        m = 0.5 * (a + b)
        lm = 0.5 * (a + m)
        rm = 0.5 * (m + b)
        flm, frm = f(lm), f(rm)
        left = (m - a) * (fa + 4.0 * flm + fm) / 6.0
        right = (b - m) * (fm + 4.0 * frm + fb) / 6.0
        delta = left + right - whole
        if depth <= 0:
            return left + right + delta / 15.0, False
        if abs(delta) <= 15.0 * tol:
            return left + right + delta / 15.0, True
        lv, lok = recurse(a, m, fa, flm, fm, left, 0.5 * tol, depth - 1)
        rv, rok = recurse(m, b, fm, frm, fb, right, 0.5 * tol, depth - 1)
        return lv + rv, lok and rok

    value, ok = recurse(a, b, fa, fm, fb, whole, tol, max_depth)
    return QuadratureResult(value, ok, evaluations)


def _check_finite(name, value):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ValueError(f"{name} must be a real number, got {value!r}")
    if not math.isfinite(value):
        raise ValueError(f"{name} must be finite, got {value!r}")
    return float(value)


@dataclass
class TransitModel:
    """Quadratic limb-darkened star (unit radius) plus opaque planet.

    I(mu) = 1 - u1 (1 - mu) - u2 (1 - mu)^2, mu = sqrt(1 - r^2).
    The planet moves on a straight line at constant projected velocity.
    """

    planet_radius: float
    impact_parameter: float
    t_mid_hours: float
    velocity: float
    u1: float = 0.0
    u2: float = 0.0
    tolerance: float = 1e-10
    max_depth: int = 30
    diagnostics: dict = field(default_factory=dict, init=False)

    def __post_init__(self):
        self.planet_radius = _check_finite("planet_radius", self.planet_radius)
        self.impact_parameter = _check_finite("impact_parameter", self.impact_parameter)
        self.t_mid_hours = _check_finite("t_mid_hours", self.t_mid_hours)
        self.velocity = _check_finite("velocity", self.velocity)
        self.u1 = _check_finite("u1", self.u1)
        self.u2 = _check_finite("u2", self.u2)
        self.tolerance = _check_finite("tolerance", self.tolerance)
        if not (0.0 < self.planet_radius <= 1.0):
            raise ValueError("planet_radius must be in (0, 1]")
        if self.impact_parameter < 0.0:
            raise ValueError("impact_parameter must be >= 0")
        if self.velocity <= 0.0:
            raise ValueError("velocity must be > 0")
        if self.u1 < 0.0 or self.u2 < 0.0:
            raise ValueError("limb-darkening coefficients must be >= 0")
        if self.u1 + self.u2 > 1.0:
            raise ValueError("limb-darkening coefficients must satisfy u1 + u2 <= 1")
        if self.tolerance <= 0.0:
            raise ValueError("tolerance must be > 0")
        # Rewrite I(r) = a0 + a1 mu + a2 mu^2 with mu = sqrt(1 - r^2).
        self._a0 = 1.0 - self.u1 - self.u2
        self._a1 = self.u1 + 2.0 * self.u2
        self._a2 = -self.u2
        # Total unocculted flux: pi*a0 + 2*pi/3*a1 + pi/2*a2.
        self.total_flux = math.pi * (1.0 - self.u1 / 3.0 - self.u2 / 6.0)

    def separation(self, t_hours):
        """Sky-projected star-planet separation in stellar radii."""
        x = self.velocity * (t_hours - self.t_mid_hours)
        return math.hypot(x, self.impact_parameter)

    def contact_interval(self):
        """Open interval (t1, t4) with d < 1 + R, or None if empty/tangent."""
        reach = (1.0 + self.planet_radius) ** 2 - self.impact_parameter ** 2
        if reach <= 0.0:
            return None
        half = math.sqrt(reach) / self.velocity
        return (self.t_mid_hours - half, self.t_mid_hours + half)

    def contact_times(self):
        """All contact times (d = 1+R and d = |1-R|) sorted, possibly empty."""
        times = []
        for dist in (1.0 + self.planet_radius, abs(1.0 - self.planet_radius)):
            reach = dist * dist - self.impact_parameter ** 2
            if reach > 0.0:
                half = math.sqrt(reach) / self.velocity
                times.append(self.t_mid_hours - half)
                times.append(self.t_mid_hours + half)
        times.sort()
        return times

    def _blocked_flux_uncached(self, d):
        """Flux of the star covered by a planet at separation d."""
        radius = self.planet_radius
        if d >= 1.0 + radius:
            return QuadratureResult(0.0, True, 0)
        if d <= radius - 1.0:
            # Star fully inside the planet's disk.
            return QuadratureResult(self.total_flux, True, 0)

        # Polar coordinates centred on the star. For each angle theta from
        # the star-planet axis, the overlap spans radii [r_lo, r_hi].
        sin_theta_max = min(1.0, radius / d) if d > 0.0 else 0.0
        theta_max = math.asin(sin_theta_max) if radius < d else math.pi

        a0, a1, a2 = self._a0, self._a1, self._a2

        def integrand(theta):
            sin_t = math.sin(theta)
            cos_t = math.cos(theta)
            # root = sqrt(R^2 - d^2 sin^2 theta), computed without the
            # catastrophic cancellation that occurs near theta_max:
            # R - d sin(theta) = d (sin(theta_max) - sin(theta)).
            if radius < d:
                diff = 2.0 * d * math.cos(0.5 * (theta_max + theta)) * math.sin(
                    0.5 * (theta_max - theta)
                )
            else:
                diff = radius - d * sin_t
            root = math.sqrt(max(diff * (radius + d * sin_t), 0.0))
            r_lo = max(d * cos_t - root, 0.0)
            r_hi = min(d * cos_t + root, 1.0)
            if r_hi <= r_lo:
                return 0.0
            # Radial moments of {1, mu, mu^2} over [r_lo, r_hi] in factored
            # form (width w, sum s) to avoid cancellation for thin slivers.
            w = r_hi - r_lo
            s = r_hi + r_lo
            p = 1.0 - r_lo * r_lo
            q = 1.0 - r_hi * r_hi
            moment_1 = 0.5 * w * s
            sqrt_p = math.sqrt(max(p, 0.0))
            sqrt_q = math.sqrt(max(q, 0.0))
            if sqrt_p + sqrt_q > 0.0:
                moment_mu = (
                    w * s * (p + sqrt_p * sqrt_q + q) / (3.0 * (sqrt_p + sqrt_q))
                )
            else:
                moment_mu = 0.0
            moment_mu2 = 0.5 * w * s * (1.0 - 0.5 * (r_lo * r_lo + r_hi * r_hi))
            return a0 * moment_1 + a1 * moment_mu + a2 * moment_mu2

        # Integrand is even in theta; integrate [0, theta_max] and double.
        # Tolerance is absolute in flux units, so scale by the total flux.
        tol = self.tolerance * self.total_flux
        if radius < d:
            # The integrand vanishes like sqrt(theta_max - theta) at the
            # tangency endpoint; the substitution theta = theta_max - u^2
            # removes the cusp so adaptive Simpson converges quickly.
            u_max = math.sqrt(theta_max)

            def transformed(u):
                return integrand(theta_max - u * u) * 2.0 * u

            result = adaptive_simpson(
                transformed, 0.0, u_max, tol, self.max_depth
            )
        else:
            result = adaptive_simpson(
                integrand, 0.0, theta_max, tol, self.max_depth
            )
        return QuadratureResult(
            2.0 * result.value, result.converged, result.evaluations
        )

    def blocked_flux(self, d):
        """Blocked flux at separation d; records diagnostics."""
        result = self._blocked_flux_uncached(d)
        self.diagnostics.setdefault("blocked_flux_calls", 0)
        self.diagnostics["blocked_flux_calls"] += 1
        self.diagnostics.setdefault("blocked_flux_evaluations", 0)
        self.diagnostics["blocked_flux_evaluations"] += result.evaluations
        if not result.converged:
            self.diagnostics["converged"] = False
        return result

    def instantaneous(self, t_hours):
        """Unocculted-normalized brightness (1 = no transit) at time t."""
        result = self.blocked_flux(self.separation(t_hours))
        return QuadratureResult(
            1.0 - result.value / self.total_flux,
            result.converged,
            result.evaluations,
        )

    def exposure_mean(self, t_center_hours, duration_hours):
        """Mean brightness over [t_center - dt/2, t_center + dt/2].

        Zero duration returns the instantaneous value. The interval is
        split at contact times so the integrand stays smooth per segment.
        """
        duration = _check_finite("duration_hours", duration_hours)
        if duration < 0.0:
            raise ValueError("duration_hours must be >= 0")
        if duration == 0.0:
            return self.instantaneous(t_center_hours)

        start = t_center_hours - 0.5 * duration
        end = t_center_hours + 0.5 * duration
        splits = [start]
        splits.extend(t for t in self.contact_times() if start < t < end)
        splits.append(end)

        total = 0.0
        converged = True
        evaluations = 0
        for a, b in zip(splits, splits[1:]):
            # Per-segment absolute tolerance scales with segment length so
            # the whole exposure meets the relative ``tolerance``.
            seg = adaptive_simpson(
                lambda t: self.instantaneous(t).value,
                a,
                b,
                self.tolerance * (b - a),
                self.max_depth,
            )
            total += seg.value
            converged = converged and seg.converged
            evaluations += seg.evaluations
        if not converged:
            self.diagnostics["converged"] = False
        return QuadratureResult(total / duration, converged, evaluations)
