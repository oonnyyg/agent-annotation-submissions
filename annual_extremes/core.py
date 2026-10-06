"""Core statistics for annual-maxima Gumbel analysis."""

from __future__ import annotations

import math
from dataclasses import dataclass

EULER_MASCHERONI = 0.5772156649015329

MIN_YEARS = 3


@dataclass(frozen=True)
class AnnualSeries:
    """Yearly maxima plus the missing years inside the observed span."""

    years: tuple[int, ...]
    values: tuple[float, ...]
    missing_years: tuple[int, ...]


@dataclass(frozen=True)
class FitResult:
    location: float
    scale: float
    mean: float
    stdev: float
    count: int


def build_annual_series(records: list[tuple[int, float]]) -> AnnualSeries:
    """Collapse (year, value) records to annual maxima.

    Multiple records in the same year keep the maximum. Years missing
    inside the observed span are listed but never filled with zeros.
    """
    per_year: dict[int, float] = {}
    for year, value in records:
        if not math.isfinite(value):
            raise ValueError(f"non-finite observation for year {year}: {value!r}")
        if year not in per_year or value > per_year[year]:
            per_year[year] = value
    if len(per_year) < MIN_YEARS:
        raise ValueError(
            f"need at least {MIN_YEARS} distinct years, got {len(per_year)}"
        )
    years = sorted(per_year)
    missing = tuple(y for y in range(years[0], years[-1] + 1) if y not in per_year)
    return AnnualSeries(
        years=tuple(years),
        values=tuple(per_year[y] for y in years),
        missing_years=missing,
    )


def fit_gumbel(values: tuple[float, ...] | list[float]) -> FitResult:
    """Method-of-moments Gumbel (maxima) fit.

    scale = sample stdev (ddof=1) * sqrt(6) / pi
    location = mean - Euler-Mascheroni constant * scale
    """
    n = len(values)
    if n < 2:
        raise ValueError("need at least 2 values to fit")
    mean = math.fsum(values) / n
    var = math.fsum((v - mean) ** 2 for v in values) / (n - 1)
    stdev = math.sqrt(var)
    scale = stdev * math.sqrt(6.0) / math.pi
    location = mean - EULER_MASCHERONI * scale
    return FitResult(
        location=location, scale=scale, mean=mean, stdev=stdev, count=n
    )


def gumbel_quantile(fit: FitResult, probability: float) -> float:
    """Inverse CDF of the fitted Gumbel distribution."""
    if not 0.0 < probability < 1.0:
        raise ValueError(f"probability must be in (0, 1), got {probability}")
    return fit.location - fit.scale * math.log(-math.log(probability))


def return_level(fit: FitResult, return_period: float) -> float:
    """Level for return period T > 1 via non-exceedance prob 1 - 1/T."""
    if return_period <= 1.0:
        raise ValueError(f"return period must be > 1, got {return_period}")
    return gumbel_quantile(fit, 1.0 - 1.0 / return_period)


def empirical_exceedance(
    years: tuple[int, ...], values: tuple[float, ...]
) -> list[tuple[int, float, float, float]]:
    """Rows of (year, value, average rank, empirical exceedance probability).

    Sorted by value descending; ties share the average rank.
    Exceedance probability = rank / (n + 1).
    """
    n = len(values)
    order = sorted(range(n), key=lambda i: (-values[i], years[i]))
    ranks = [0.0] * n
    i = 0
    while i < n:
        j = i
        while j + 1 < n and values[order[j + 1]] == values[order[i]]:
            j += 1
        avg_rank = (i + 1 + j + 1) / 2.0
        for k in range(i, j + 1):
            ranks[order[k]] = avg_rank
        i = j + 1
    return [
        (years[idx], values[idx], ranks[idx], ranks[idx] / (n + 1))
        for idx in order
    ]
