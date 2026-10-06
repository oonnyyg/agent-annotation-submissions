
"""Residual analysis and finite candidate ranking."""

from __future__ import annotations

from .io_config import Candidate, Config
from .model import AquiferModel


def weighted_squared_error(config: Config, candidate: Candidate) -> dict:
    """Weighted squared error of a candidate against observed drawdowns.

    The candidate parameters replace the aquifer T and S; wells, points
    and observed data come from the base config. No continuous
    optimisation is performed.
    """
    model = AquiferModel(
        transmissivity=candidate.transmissivity,
        storativity=candidate.storativity,
        wells=config.model.wells,
        tol=config.model.tol,
        max_terms=config.model.max_terms,
    )
    sse = 0.0
    weight_sum = 0.0
    count = 0
    per_series = []
    for series in config.observed:
        point = next(p for p in config.points if p.name == series.point)
        residuals = []
        for t, obs, w in zip(series.times, series.drawdowns, series.weights):
            sim = sum(
                model.well_drawdown(well, point.x, point.y, t)
                for well in model.wells
            )
            res = sim - obs
            sse += w * res * res
            weight_sum += w
            count += 1
            residuals.append(
                {"time": t, "observed": obs, "simulated": sim,
                 "residual": res, "weight": w}
            )
        per_series.append({"point": series.point, "residuals": residuals})
    return {
        "name": candidate.name,
        "transmissivity": candidate.transmissivity,
        "storativity": candidate.storativity,
        "weighted_squared_error": sse,
        "weighted_root_mean_square": (
            (sse / weight_sum) ** 0.5 if weight_sum > 0.0 else None
        ),
        "sample_count": count,
        "series": per_series,
    }


def rank_candidates(config: Config) -> list[dict]:
    """Evaluate every finite candidate and sort by weighted squared error.

    Ties keep the input order (Python's sort is stable). Raises
    ConfigError/ModelError if a candidate has non-positive parameters.
    """
    results = [weighted_squared_error(config, c) for c in config.candidates]
    results.sort(key=lambda r: r["weighted_squared_error"])
    for rank, result in enumerate(results, start=1):
        result["rank"] = rank
    return results
