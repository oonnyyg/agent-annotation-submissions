"""Period-by-period multi-source plume accounting and receptor summaries."""

from __future__ import annotations

from dataclasses import dataclass

from .config import MetPeriod, Scenario
from .model import concentration, wind_relative_coordinates


@dataclass(frozen=True)
class Contribution:
    period_index: int
    period_start: str
    period_end: str
    receptor_id: str
    source_id: str
    concentration: float  # mass per cubic metre


@dataclass(frozen=True)
class ReceptorSummary:
    receptor_id: str
    total_duration_s: float
    time_integral: float  # concentration * seconds
    weighted_average: float  # mass per cubic metre
    peak_concentration: float
    peak_period_index: int
    peak_period_start: str


def compute_contributions(
    scenario: Scenario, periods: tuple[MetPeriod, ...]
) -> list[Contribution]:
    """Concentration of every (period, receptor, source) combination."""
    contributions: list[Contribution] = []
    for period_index, period in enumerate(periods):
        for receptor in scenario.receptors:
            for source in scenario.sources:
                downwind, crosswind = wind_relative_coordinates(
                    receptor.east_m - source.east_m,
                    receptor.north_m - source.north_m,
                    period.wind_from_deg,
                )
                value = concentration(
                    emission_rate=source.emission_rate,
                    wind_speed=period.wind_speed,
                    downwind_m=downwind,
                    crosswind_m=crosswind,
                    source_height_m=source.height_m,
                    receptor_height_m=receptor.height_m,
                    coeffs=scenario.coeffs,
                    decay_rate=scenario.decay_rate,
                )
                contributions.append(
                    Contribution(
                        period_index=period_index,
                        period_start=period.start,
                        period_end=period.end,
                        receptor_id=receptor.id,
                        source_id=source.id,
                        concentration=value,
                    )
                )
    return contributions


def summarize(
    scenario: Scenario,
    periods: tuple[MetPeriod, ...],
    contributions: list[Contribution],
) -> list[ReceptorSummary]:
    """Duration-weighted receptor summaries over valid met periods.

    Gaps between periods are excluded (not treated as zero-concentration
    time). Peak ties resolve to the earliest period.
    """
    totals: dict[str, float] = {r.id: 0.0 for r in scenario.receptors}
    durations: dict[str, float] = {r.id: 0.0 for r in scenario.receptors}
    peak: dict[str, tuple[float, int, str]] = {
        r.id: (float("-inf"), -1, "") for r in scenario.receptors
    }
    per_period: dict[tuple[int, str], float] = {}
    for item in contributions:
        key = (item.period_index, item.receptor_id)
        per_period[key] = per_period.get(key, 0.0) + item.concentration

    for period_index, period in enumerate(periods):
        for receptor in scenario.receptors:
            value = per_period.get((period_index, receptor.id), 0.0)
            totals[receptor.id] += value * period.duration_s
            durations[receptor.id] += period.duration_s
            best = peak[receptor.id]
            if value > best[0]:  # strict: earliest period wins ties
                peak[receptor.id] = (value, period_index, period.start)

    summaries: list[ReceptorSummary] = []
    for receptor in scenario.receptors:
        duration = durations[receptor.id]
        best_value, best_index, best_start = peak[receptor.id]
        summaries.append(
            ReceptorSummary(
                receptor_id=receptor.id,
                total_duration_s=duration,
                time_integral=totals[receptor.id],
                weighted_average=totals[receptor.id] / duration if duration else 0.0,
                peak_concentration=best_value,
                peak_period_index=best_index,
                peak_period_start=best_start,
            )
        )
    return summaries
