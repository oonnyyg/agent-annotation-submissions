"""Ideal confined-aquifer model: Theis superposition in time and space.

Assumptions: homogeneous, isotropic, infinite areal extent, constant
transmissivity T (m^2/s) and storativity S (-). Each well has a
piecewise-constant rate schedule (m^3/s, positive = pumping,
negative = injection); the rate is zero before the first schedule entry.

Drawdown at distance r and time t from one rate step dQ starting at t_j:

    s = dQ / (4*pi*T) * W(u),   u = r^2 * S / (4 * T * (t - t_j))

Responses of every rate increment of every well are summed. A rate change
at time t_j contributes nothing exactly at t_j (the new increment has
zero elapsed time); shutdown is modelled as a negative increment, so the
pre-shutdown history keeps contributing during recovery.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .wellfunction import DEFAULT_MAX_TERMS, DEFAULT_TOLERANCE, well_function


class ModelError(Exception):
    """Raised for invalid model configuration."""


@dataclass(frozen=True)
class RateStep:
    time: float
    rate: float


@dataclass(frozen=True)
class Well:
    name: str
    x: float
    y: float
    schedule: tuple[RateStep, ...]


@dataclass(frozen=True)
class ObservationPoint:
    name: str
    x: float
    y: float


@dataclass(frozen=True)
class Aquifer:
    transmissivity: float
    storativity: float

    def __post_init__(self) -> None:
        if not math.isfinite(self.transmissivity) or self.transmissivity <= 0.0:
            raise ModelError("transmissivity must be a positive finite number (m^2/s)")
        if not math.isfinite(self.storativity) or self.storativity <= 0.0:
            raise ModelError("storativity must be a positive finite number (-)")


def validate_geometry(wells: list[Well], points: list[ObservationPoint]) -> None:
    for point in points:
        for well in wells:
            if point.x == well.x and point.y == well.y:
                raise ModelError(
                    f"observation point {point.name!r} coincides with well {well.name!r}"
                )


def _rate_increments(well: Well) -> list[tuple[float, float]]:
    """(time, delta_rate) pairs; rate is zero before the first entry."""
    increments: list[tuple[float, float]] = []
    previous = 0.0
    for step in well.schedule:
        delta = step.rate - previous
        if delta != 0.0:
            increments.append((step.time, delta))
        previous = step.rate
    return increments


@dataclass
class WellFunctionConfig:
    tolerance: float = DEFAULT_TOLERANCE
    max_terms: int = DEFAULT_MAX_TERMS


@dataclass
class EvaluationStats:
    evaluations: int = 0
    max_terms_used: int = 0
    methods: dict[str, int] = field(default_factory=dict)

    def record(self, method: str, terms: int) -> None:
        self.evaluations += 1
        self.max_terms_used = max(self.max_terms_used, terms)
        self.methods[method] = self.methods.get(method, 0) + 1


def well_contribution_at(
    aquifer: Aquifer,
    well: Well,
    point: ObservationPoint,
    time: float,
    config: WellFunctionConfig | None = None,
    stats: EvaluationStats | None = None,
) -> float:
    """Drawdown (m) at one point and time caused by a single well.

    Pure function of the inputs: repeated calls and additional sample
    times never change results at already-computed times.
    """
    config = config or WellFunctionConfig()
    r2 = (point.x - well.x) ** 2 + (point.y - well.y) ** 2
    coef_u = r2 * aquifer.storativity / (4.0 * aquifer.transmissivity)
    total = 0.0
    for step_time, delta_q in _rate_increments(well):
        elapsed = time - step_time
        if elapsed <= 0.0:
            # A rate change contributes zero at its switch instant and
            # nothing before it.
            continue
        u = coef_u / elapsed
        result = well_function(u, config.tolerance, config.max_terms)
        if stats is not None:
            stats.record(result.method, result.terms)
        total += delta_q / (4.0 * math.pi * aquifer.transmissivity) * result.value
    return total


def simulate(
    aquifer: Aquifer,
    wells: list[Well],
    points: list[ObservationPoint],
    times: list[float],
    config: WellFunctionConfig | None = None,
    stats: EvaluationStats | None = None,
) -> dict[str, dict[str, list[float]]]:
    """Drawdown time series per point, per well, plus totals.

    Returns {point_name: {"total": [...], well_name: [...]}} aligned with
    the given times.
    """
    validate_geometry(wells, points)
    series: dict[str, dict[str, list[float]]] = {}
    for point in points:
        per_well: dict[str, list[float]] = {}
        totals: list[float] = []
        for well in wells:
            values = [
                well_contribution_at(aquifer, well, point, t, config, stats)
                for t in times
            ]
            per_well[well.name] = values
            if totals:
                totals = [a + b for a, b in zip(totals, values)]
            else:
                totals = list(values)
        per_well["total"] = totals
        series[point.name] = per_well
    return series
