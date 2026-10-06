"""Bootstrap resampling for return-level uncertainty."""

from __future__ import annotations

import random
from dataclasses import dataclass

from .core import fit_gumbel, return_level


@dataclass(frozen=True)
class BootstrapCI:
    return_period: float
    lower: float
    upper: float
    degenerate_count: int


def percentile_linear(sorted_values: list[float], p: float) -> float:
    """Percentile with linear interpolation (position p * (n - 1))."""
    n = len(sorted_values)
    if n == 1:
        return sorted_values[0]
    pos = p * (n - 1)
    lo = int(pos)
    hi = min(lo + 1, n - 1)
    frac = pos - lo
    return sorted_values[lo] + frac * (sorted_values[hi] - sorted_values[lo])


def bootstrap_return_levels(
    values: tuple[float, ...],
    return_periods: list[float],
    resamples: int,
    seed: int,
    alpha: float = 0.05,
) -> list[BootstrapCI]:
    """Resample annual maxima with replacement and refit per replicate.

    All return periods are computed for every replicate, so the order of
    ``return_periods`` cannot affect results. Replicates whose sample is
    entirely constant yield that constant as a degenerate quantile and
    are counted, never discarded or redrawn.
    """
    n = len(values)
    rng = random.Random(seed)
    draws: dict[float, list[float]] = {t: [] for t in return_periods}
    degenerate = 0
    for _ in range(resamples):
        sample = [values[rng.randrange(n)] for _ in range(n)]
        if all(v == sample[0] for v in sample):
            degenerate += 1
            for t in return_periods:
                draws[t].append(sample[0])
            continue
        fit = fit_gumbel(sample)
        for t in return_periods:
            draws[t].append(return_level(fit, t))
    results = []
    for t in return_periods:
        ordered = sorted(draws[t])
        results.append(
            BootstrapCI(
                return_period=t,
                lower=percentile_linear(ordered, alpha / 2.0),
                upper=percentile_linear(ordered, 1.0 - alpha / 2.0),
                degenerate_count=degenerate,
            )
        )
    return results
