
"""Confined-aquifer multi-well superposition model.

Assumptions: homogeneous, isotropic, infinite areal extent, confined
aquifer with constant transmissivity T (m^2/s) and storativity S (-).
Each well has a piecewise-constant pumping schedule (m^3/s); positive
values are pumping, negative values injection. The rate before the first
schedule time is zero. Responses of rate increments are computed with the
Theis solution and superposed in time and space. A rate change at time t_c
contributes zero exactly at t_c; stopping a well is a negative increment
and does not erase the history of earlier increments.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .well_function import theis_w


class ModelError(Exception):
    """Raised for invalid model configuration (bad parameters, geometry)."""


@dataclass(frozen=True)
class Well:
    name: str
    x: float
    y: float
    # schedule: sorted list of (time_s, rate_m3s); rate applies for t >= time.
    schedule: tuple[tuple[float, float], ...]


@dataclass(frozen=True)
class ObservationPoint:
    name: str
    x: float
    y: float


@dataclass
class AquiferModel:
    transmissivity: float  # m^2/s
    storativity: float     # dimensionless
    wells: list[Well] = field(default_factory=list)
    tol: float = 1e-12
    max_terms: int = 10000

    def __post_init__(self) -> None:
        if not math.isfinite(self.transmissivity) or self.transmissivity <= 0.0:
            raise ModelError(
                f"transmissivity must be positive, got {self.transmissivity!r}"
            )
        if not math.isfinite(self.storativity) or self.storativity <= 0.0:
            raise ModelError(
                f"storativity must be positive, got {self.storativity!r}"
            )
        for well in self.wells:
            times = [t for t, _ in well.schedule]
            if any(t2 <= t1 for t1, t2 in zip(times, times[1:])):
                raise ModelError(
                    f"well {well.name!r}: schedule times must be strictly increasing"
                )

    def validate_points(self, points: list[ObservationPoint]) -> None:
        for point in points:
            for well in self.wells:
                if point.x == well.x and point.y == well.y:
                    raise ModelError(
                        f"observation point {point.name!r} coincides with "
                        f"well {well.name!r} at ({point.x}, {point.y})"
                    )

    def well_drawdown(self, well: Well, x: float, y: float, t: float) -> float:
        """Drawdown (m, positive = water level down) at (x, y) and time t
        caused by a single well."""
        r2 = (x - well.x) ** 2 + (y - well.y) ** 2
        if r2 == 0.0:
            raise ModelError(
                f"evaluation point coincides with well {well.name!r}"
            )
        total = 0.0
        coef = r2 * self.storativity / (4.0 * self.transmissivity)
        schedule = well.schedule
        for i, (t0, q0) in enumerate(schedule):
            if t < t0:
                break
            dq = q0 - (schedule[i - 1][1] if i > 0 else 0.0)
            if dq == 0.0:
                continue
            dt = t - t0
            if dt == 0.0:
                continue  # a new increment contributes zero at its switch time
            u = coef / dt
            total += dq / (4.0 * math.pi * self.transmissivity) * theis_w(
                u, self.tol, self.max_terms
            )
        return total

    def contributions(
        self, points: list[ObservationPoint], times: list[float]
    ) -> dict[str, dict[str, list[float]]]:
        """Drawdown time series per observation point and per well.

        Returns {point_name: {well_name: [values...]}}. Evaluation is
        stateless: repeated calls or extra sample times never change the
        values at previously computed times.
        """
        self.validate_points(points)
        result: dict[str, dict[str, list[float]]] = {}
        for point in points:
            per_well: dict[str, list[float]] = {}
            for well in self.wells:
                per_well[well.name] = [
                    self.well_drawdown(well, point.x, point.y, t) for t in times
                ]
            result[point.name] = per_well
        return result

    def total_drawdown(
        self, points: list[ObservationPoint], times: list[float]
    ) -> dict[str, list[float]]:
        """Total drawdown per observation point (sum over wells)."""
        contrib = self.contributions(points, times)
        return {
            name: [sum(vals) for vals in zip(*per_well.values())]
            if per_well
            else [0.0] * len(times)
            for name, per_well in contrib.items()
        }
