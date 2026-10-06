"""Multi-source, multi-period exposure accounting."""

from __future__ import annotations

from dataclasses import dataclass

from .io import Config, MetPeriod
from .model import concentration, wind_components


@dataclass(frozen=True)
class Contribution:
    period_index: int
    source_id: str
    receptor_id: str
    concentration: float  # mass/m^3 (per emission mass units)
    time_integral: float  # concentration * seconds


@dataclass(frozen=True)
class ReceptorSummary:
    receptor_id: str
    total_duration: float
    average_concentration: float
    time_integral: float
    peak_concentration: float
    peak_period_index: int
    peak_period_start: str


def compute_contributions(
    config: Config, periods: list[MetPeriod]
) -> list[Contribution]:
    """Per-period, per-source, per-receptor steady-state concentrations."""
    disp = config.dispersion
    contributions: list[Contribution] = []
    for pidx, period in enumerate(periods):
        for src in config.sources:
            for rec in config.receptors:
                downwind, crosswind = wind_components(
                    rec.easting - src.easting,
                    rec.northing - src.northing,
                    period.wind_dir,
                )
                conc = concentration(
                    emission_rate=src.emission_rate,
                    wind_speed=period.wind_speed,
                    source_height=src.height,
                    receptor_height=rec.height,
                    downwind=downwind,
                    crosswind=crosswind,
                    sigma_y_coeff=disp.sigma_y_coeff,
                    sigma_y_exp=disp.sigma_y_exp,
                    sigma_z_coeff=disp.sigma_z_coeff,
                    sigma_z_exp=disp.sigma_z_exp,
                    decay_rate=config.decay_rate,
                )
                contributions.append(
                    Contribution(
                        period_index=pidx,
                        source_id=src.id,
                        receptor_id=rec.id,
                        concentration=conc,
                        time_integral=conc * period.duration,
                    )
                )
    return contributions


def summarize(
    config: Config, periods: list[MetPeriod], contributions: list[Contribution]
) -> list[ReceptorSummary]:
    """Duration-weighted receptor summaries over all sources and periods.

    Gaps between periods do not count as zero-concentration time.
    Peak ties resolve to the earliest period.
    """
    totals: dict[str, float] = {rec.id: 0.0 for rec in config.receptors}
    durations: dict[str, float] = {rec.id: 0.0 for rec in config.receptors}
    peak: dict[str, tuple[float, int]] = {
        rec.id: (-1.0, -1) for rec in config.receptors
    }

    period_totals: dict[tuple[int, str], float] = {}
    for c in contributions:
        totals[c.receptor_id] += c.time_integral
        key = (c.period_index, c.receptor_id)
        period_totals[key] = period_totals.get(key, 0.0) + c.concentration

    for pidx, period in enumerate(periods):
        for rec in config.receptors:
            durations[rec.id] += period.duration
            total = period_totals.get((pidx, rec.id), 0.0)
            best, _ = peak[rec.id]
            if total > best:
                peak[rec.id] = (total, pidx)

    summaries = []
    for rec in config.receptors:
        duration = durations[rec.id]
        integral = totals[rec.id]
        best, best_idx = peak[rec.id]
        summaries.append(
            ReceptorSummary(
                receptor_id=rec.id,
                total_duration=duration,
                average_concentration=integral / duration if duration > 0 else 0.0,
                time_integral=integral,
                peak_concentration=max(best, 0.0),
                peak_period_index=best_idx,
                peak_period_start=(
                    periods[best_idx].start.isoformat() if best_idx >= 0 else ""
                ),
            )
        )
    return summaries
