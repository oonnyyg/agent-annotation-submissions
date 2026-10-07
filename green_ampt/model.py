"""Green-Ampt infiltration core.

Assumptions: homogeneous soil, sharp wetting front, no surface storage,
no evaporation, no redistribution. Times in hours, depths in mm,
rates in mm/h.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field


@dataclass(frozen=True)
class SoilParams:
    ks: float           # saturated hydraulic conductivity, mm/h
    psi: float          # wetting-front suction head, mm
    delta_theta: float  # initial moisture deficit, dimensionless, (0, 1]

    def __post_init__(self) -> None:
        if self.ks <= 0:
            raise ValueError("ks must be positive")
        if self.psi <= 0:
            raise ValueError("psi must be positive")
        if not (0.0 < self.delta_theta <= 1.0):
            raise ValueError("delta_theta must be in (0, 1]")


@dataclass(frozen=True)
class Segment:
    start: float  # hours
    end: float    # hours
    rate: float   # rainfall intensity, mm/h


@dataclass(frozen=True)
class Event:
    time: float             # hours
    kind: str               # 'segment_start', 'ponding', 'segment_end'
    cum_rain: float         # mm
    cum_infiltration: float # mm
    cum_runoff: float       # mm
    infil_rate: float       # mm/h, infiltration rate just after this time


@dataclass
class SimulationResult:
    events: list[Event] = field(default_factory=list)

    @property
    def total_rain(self) -> float:
        return self.events[-1].cum_rain if self.events else 0.0

    @property
    def total_infiltration(self) -> float:
        return self.events[-1].cum_infiltration if self.events else 0.0

    @property
    def total_runoff(self) -> float:
        return self.events[-1].cum_runoff if self.events else 0.0


def capacity(params: SoilParams, cum_infiltration: float) -> float:
    """Green-Ampt capacity fp = Ks*(1 + psi*dtheta/F), mm/h.

    At F = 0 the capacity limit is infinite; math.inf is returned so the
    F = 0 state never divides by zero.
    """
    if cum_infiltration <= 0.0:
        return math.inf
    return params.ks * (1.0 + params.psi * params.delta_theta / cum_infiltration)


def _time_at(params: SoilParams, t0: float, f0: float, f: float) -> float:
    """Time at which cumulative infiltration reaches f under ponding.

    Inverts the cumulative Green-Ampt relation
        t - t0 = [F - F0 - M*ln((M + F)/(M + F0))] / Ks,  M = psi*dtheta.
    """
    m = params.psi * params.delta_theta
    return t0 + (f - f0 - m * math.log((m + f) / (m + f0))) / params.ks


def solve_cum_infiltration(
    params: SoilParams,
    t0: float,
    f0: float,
    t_end: float,
    tol: float = 1e-9,
    max_iter: int = 200,
) -> float:
    """Solve cumulative infiltration at t_end under ponded conditions.

    Bisection on the monotonic cumulative relation. Raises RuntimeError
    if the bracket does not shrink below tol within max_iter iterations.
    """
    if t_end <= t0:
        return f0
    # capacity(f0) may be infinite at f0 = 0, so seed the bracket with Ks
    lo = f0
    hi = f0 + params.ks * (t_end - t0)
    while _time_at(params, t0, f0, hi) < t_end:
        hi = f0 + 2.0 * (hi - f0)
    for _ in range(max_iter):
        mid = 0.5 * (lo + hi)
        if hi - lo <= tol:
            return mid
        if _time_at(params, t0, f0, mid) < t_end:
            lo = mid
        else:
            hi = mid
    raise RuntimeError(
        f"Green-Ampt solver did not converge within {max_iter} iterations"
    )

def simulate(
    params: SoilParams,
    segments: list[Segment],
    tol: float = 1e-9,
    max_iter: int = 200,
) -> SimulationResult:
    """Replay piecewise-constant rainfall through the Green-Ampt model.

    Excess rainfall becomes runoff immediately; dry segments leave the
    cumulative infiltration state untouched.
    """
    result = SimulationResult()
    f = 0.0           # cumulative infiltration, mm
    cum_rain = 0.0
    cum_runoff = 0.0
    ponded = False    # capacity-limited since a previous segment

    result.events.append(
        Event(segments[0].start, "segment_start", 0.0, 0.0, 0.0, 0.0)
    )

    for seg in segments:
        remaining = seg.end - seg.start
        rate = seg.rate
        t_phase = seg.start  # start time of the current phase

        if rate > 0.0 and not ponded:
            if rate >= capacity(params, f):
                ponded = True
            elif rate > params.ks:
                # potential ponding: Fp where capacity falls to the rain rate;
                # rates <= Ks can never pond since capacity >= Ks always
                m = params.psi * params.delta_theta
                fp = m / (rate / params.ks - 1.0)
                dt_pond = (fp - f) / rate
                if dt_pond < remaining:
                    # supply-limited until the ponding time inside the segment
                    f = fp
                    cum_rain += rate * dt_pond
                    ponded = True
                    t_phase = seg.start + dt_pond
                    remaining -= dt_pond
                    result.events.append(
                        Event(t_phase, "ponding", cum_rain, f, cum_runoff, rate)
                    )
                else:
                    f += rate * remaining
                    cum_rain += rate * remaining
                    remaining = 0.0
            else:
                # rain rate <= Ks: supply-limited for the whole segment
                f += rate * remaining
                cum_rain += rate * remaining
                remaining = 0.0
        elif rate <= 0.0:
            remaining = 0.0  # dry period: state unchanged

        if ponded and remaining > 0.0:
            f_new = solve_cum_infiltration(
                params, t_phase, f, seg.end, tol, max_iter
            )
            rain_seg = rate * remaining
            cum_rain += rain_seg
            cum_runoff += rain_seg - (f_new - f)
            f = f_new

        if rate > 0.0:
            infil_rate = rate if not ponded else capacity(params, f)
        else:
            infil_rate = 0.0
        result.events.append(
            Event(seg.end, "segment_end", cum_rain, f, cum_runoff, infil_rate)
        )

    return result