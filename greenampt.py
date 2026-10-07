"""Green-Ampt infiltration model for piecewise-constant rainfall intensity.

Units: time in hours, depths in mm, hydraulic conductivity in mm/h.
"""

import math
from dataclasses import dataclass


class SolverError(Exception):
    """Raised when the cumulative-infiltration solve fails to converge."""


@dataclass(frozen=True)
class SoilParams:
    ks: float           # saturated hydraulic conductivity [mm/h]
    psi: float          # wetting-front suction head [mm]
    delta_theta: float  # moisture deficit (porosity - initial water content) [-]

    def __post_init__(self):
        if self.ks <= 0:
            raise ValueError("saturated conductivity ks must be positive")
        if self.psi <= 0:
            raise ValueError("wetting-front suction psi must be positive")
        if not (0 < self.delta_theta <= 1):
            raise ValueError("delta_theta must be in (0, 1]")

    @property
    def suction_moisture(self):
        """Product psi * delta_theta [mm], the Green-Ampt storage scale."""
        return self.psi * self.delta_theta


def infiltration_capacity(soil, cum_infiltration):
    """Capacity fp = Ks * (1 + psi*dtheta / F) [mm/h].

    At F == 0 the limit is +inf: any finite intensity infiltrates fully
    until the surface ponds.
    """
    if cum_infiltration <= 0.0:
        return float("inf")
    return soil.ks * (1.0 + soil.suction_moisture / cum_infiltration)


def ponding_cumulative(soil, intensity):
    """Cumulative F_p at which capacity falls to `intensity` [mm].

    Returns +inf when the intensity can never exceed capacity (i <= Ks).
    """
    if intensity <= soil.ks:
        return float("inf")
    return soil.ks * soil.suction_moisture / (intensity - soil.ks)


def cumulative_time(soil, f, f_pond, t_pond):
    """Elapsed time to reach cumulative infiltration f after ponding [h].

    Implicit Green-Ampt cumulative relation anchored at the ponding state:
        Ks*(t - t_pond) = (f - f_pond) - M * ln((f + M) / (f_pond + M))
    with M = psi * delta_theta.
    """
    m = soil.suction_moisture
    return t_pond + ((f - f_pond) - m * math.log((f + m) / (f_pond + m))) / soil.ks


def solve_cumulative(soil, t_target, f_pond, t_pond, tol=1e-9, max_iter=100):
    """Solve for cumulative infiltration F at time t_target (post-ponding).

    Newton iteration with a bisection safeguard. Raises SolverError if the
    iteration does not converge within max_iter steps.
    """
    m = soil.suction_moisture
    ks_dt = soil.ks * (t_target - t_pond)
    if ks_dt <= 0.0:
        return f_pond

    lo = f_pond
    hi = f_pond + ks_dt + m
    while cumulative_time(soil, hi, f_pond, t_pond) < t_target:
        hi = f_pond + 2.0 * (hi - f_pond) + m

    f = lo + ks_dt
    for _ in range(max_iter):
        g = (f - f_pond) - m * math.log((f + m) / (f_pond + m)) - ks_dt
        if abs(g) <= tol * max(1.0, ks_dt):
            return f
        dg = f / (f + m)  # dG/dF = 1 - m/(f + m)
        if dg > 0:
            f_new = f - g / dg
            if lo < f_new < hi:
                f = f_new
                continue
        if g > 0:
            hi = f
        else:
            lo = f
        f = 0.5 * (lo + hi)
    raise SolverError(
        "cumulative infiltration solve did not converge in "
        f"{max_iter} iterations (dt={t_target - t_pond:.6g} h, "
        f"f_pond={f_pond:.6g} mm)"
    )


@dataclass(frozen=True)
class Segment:
    """One rainfall segment: [t_start, t_end) with constant intensity [mm/h]."""
    t_start: float
    t_end: float
    intensity: float


@dataclass(frozen=True)
class Event:
    """A row of the output process table (state at time t)."""
    t: float
    kind: str            # 'start', 'segment_end', or 'ponding'
    intensity: float   # rainfall intensity applicable just after t [mm/h]
    cum_infiltration: float
    cum_runoff: float
    infiltration_rate: float  # rate applicable just after t [mm/h]


def simulate(segments, soil, tol=1e-9, max_iter=100):
    """Replay piecewise-constant rainfall through the Green-Ampt model.

    Returns a list of Event rows at every segment boundary and at any
    intra-segment ponding transition. Excess rainfall becomes runoff
    instantaneously (no surface storage, no evaporation, no redistribution).
    """
    events = []
    cum_f = 0.0
    cum_r = 0.0
    t_now = segments[0].t_start if segments else 0.0
    events.append(Event(t_now, "start",
                        segments[0].intensity if segments else 0.0,
                        0.0, 0.0,
                        segments[0].intensity if segments else 0.0))

    for seg in segments:
        if seg.t_start != t_now:
            raise ValueError(
                f"gap or overlap between segments at t={seg.t_start}")
        if seg.t_end <= seg.t_start:
            raise ValueError(f"non-positive duration at t={seg.t_start}")
        if seg.intensity < 0:
            raise ValueError(f"negative intensity at t={seg.t_start}")

        i = seg.intensity
        f_p = ponding_cumulative(soil, i)
        f_start = cum_f

        if infiltration_capacity(soil, cum_f) <= i:
            # Already infiltration-limited at segment start.
            f_end = solve_cumulative(soil, seg.t_end, cum_f, seg.t_start,
                                     tol, max_iter)
        elif cum_f + i * (seg.t_end - seg.t_start) <= f_p:
            # Supply-limited for the whole segment.
            f_end = cum_f + i * (seg.t_end - seg.t_start)
        else:
            # Ponding occurs inside this segment: rain fully infiltrates
            # until F reaches f_p, then the capacity relation takes over.
            t_pond = seg.t_start + (f_p - cum_f) / i
            events.append(Event(t_pond, "ponding", i, f_p, cum_r, i))
            f_end = solve_cumulative(soil, seg.t_end, f_p, t_pond,
                                     tol, max_iter)

        added = f_end - f_start
        cum_r += i * (seg.t_end - seg.t_start) - added
        cum_f = f_end
        t_now = seg.t_end
        rate = min(i, infiltration_capacity(soil, cum_f))
        events.append(Event(t_now, "segment_end", i, cum_f, cum_r, rate))

    return events


def totals(segments, events):
    """Return (total_rain, total_infiltration, total_runoff) in mm."""
    rain = sum(s.intensity * (s.t_end - s.t_start) for s in segments)
    return rain, events[-1].cum_infiltration, events[-1].cum_runoff
