"""CSV/JSON input-output helpers."""

from __future__ import annotations

import csv
import json
import math
from pathlib import Path

from .bootstrap import BootstrapCI
from .core import AnnualSeries, FitResult


def read_observations(path: str | Path) -> list[tuple[int, float]]:
    """Read a CSV with header columns ``year,value``."""
    records: list[tuple[int, float]] = []
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.DictReader(fh)
        if reader.fieldnames is None or not {"year", "value"} <= set(
            reader.fieldnames
        ):
            raise ValueError("observations CSV needs 'year' and 'value' columns")
        for row in reader:
            records.append((int(row["year"]), float(row["value"])))
    if not records:
        raise ValueError("observations CSV is empty")
    return records


def read_config(path: str | Path) -> dict:
    """Read JSON config: return_periods, bootstrap resamples/seed."""
    with open(path, encoding="utf-8") as fh:
        config = json.load(fh)
    periods = config.get("return_periods")
    if not periods or not all(
        isinstance(t, (int, float)) and t > 1 for t in periods
    ):
        raise ValueError("config 'return_periods' must be a list of numbers > 1")
    bootstrap = config.get("bootstrap", {})
    resamples = int(bootstrap.get("resamples", 1000))
    seed = int(bootstrap.get("seed", 0))
    if resamples < 1:
        raise ValueError("bootstrap resamples must be >= 1")
    return {
        "return_periods": [float(t) for t in periods],
        "resamples": resamples,
        "seed": seed,
    }


def write_series_csv(
    path: str | Path,
    empirical: list[tuple[int, float, float, float]],
) -> None:
    """Write annual maxima with ranks and empirical exceedance probabilities."""
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["year", "value", "rank", "exceedance_probability"])
        for year, value, rank, prob in empirical:
            writer.writerow([year, repr(value), repr(rank), repr(prob)])


def _finite(value: float) -> float:
    if not math.isfinite(value):
        raise ValueError(f"non-finite result: {value!r}")
    return value


def write_stats_json(
    path: str | Path,
    series: AnnualSeries,
    fit: FitResult,
    levels: list[tuple[float, float]],
    intervals: list[BootstrapCI],
    resamples: int,
    seed: int,
) -> None:
    """Write the statistics report JSON; every number must be finite."""
    ci_by_period = {ci.return_period: ci for ci in intervals}
    payload = {
        "n_years": len(series.years),
        "year_span": [series.years[0], series.years[-1]],
        "missing_years": list(series.missing_years),
        "fit": {
            "distribution": "gumbel_max",
            "location": _finite(fit.location),
            "scale": _finite(fit.scale),
            "mean": _finite(fit.mean),
            "stdev": _finite(fit.stdev),
        },
        "return_levels": [
            {
                "return_period": t,
                "level": _finite(level),
                "ci95": {
                    "lower": _finite(ci_by_period[t].lower),
                    "upper": _finite(ci_by_period[t].upper),
                    "method": "bootstrap_percentile_linear_interpolation",
                    "note": "pointwise interval, not a simultaneous confidence band",
                },
            }
            for t, level in levels
        ],
        "bootstrap": {
            "resamples": resamples,
            "seed": seed,
            "degenerate_resamples": intervals[0].degenerate_count if intervals else 0,
        },
    }
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=2, allow_nan=False)
        fh.write("\n")
