"""Bootstrap resampling of annual maxima with degenerate-sample handling."""

import random

from .gumbel import fit_gumbel, return_level


def percentile(sorted_values, q):
    """Linear-interpolation percentile (q in [0, 100])."""
    if not sorted_values:
        raise ValueError("empty sample")
    if len(sorted_values) == 1:
        return sorted_values[0]
    pos = (len(sorted_values) - 1) * (q / 100.0)
    lo = int(pos)
    hi = min(lo + 1, len(sorted_values) - 1)
    frac = pos - lo
    return sorted_values[lo] + frac * (sorted_values[hi] - sorted_values[lo])


def bootstrap_return_levels(values, return_periods, resamples, seed,
                            ci=(2.5, 97.5)):
    """Resample with replacement, refit, and build percentile CIs.

    Each resample computes all return periods together, so input order of
    return periods cannot affect results. Resamples whose values are all
    equal yield a degenerate (constant) quantile and are counted, not
    discarded.
    """
    rng = random.Random(seed)
    n = len(values)
    periods = list(return_periods)
    draws = {t: [] for t in periods}
    degenerate_count = 0
    for _ in range(resamples):
        sample = [values[rng.randrange(n)] for _ in range(n)]
        if all(v == sample[0] for v in sample):
            degenerate_count += 1
            for t in periods:
                draws[t].append(sample[0])
            continue
        fit = fit_gumbel(sample)
        for t in periods:
            draws[t].append(return_level(fit, t))
    intervals = {}
    for t in periods:
        ordered = sorted(draws[t])
        intervals[t] = {
            "lower": percentile(ordered, ci[0]),
            "upper": percentile(ordered, ci[1]),
        }
    return {"intervals": intervals, "degenerate_resamples": degenerate_count}
