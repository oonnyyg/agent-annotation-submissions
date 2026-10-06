"""Gumbel (maxima) fitting, return-level quantiles and empirical ranks."""

import math

EULER_MASCHERONI = 0.5772156649015329


def fit_gumbel(values):
    """Method-of-moments fit for a maxima Gumbel distribution.

    scale = sample_std * sqrt(6) / pi   (std uses denominator n - 1)
    location = mean - euler_constant * scale
    """
    n = len(values)
    if n < 2:
        raise ValueError("need at least two values to fit")
    mean = sum(values) / n
    var = sum((v - mean) ** 2 for v in values) / (n - 1)
    scale = math.sqrt(var) * math.sqrt(6.0) / math.pi
    location = mean - EULER_MASCHERONI * scale
    return {"location": location, "scale": scale, "mean": mean}


def return_level(fit, return_period):
    """Quantile at non-exceedance probability 1 - 1/T for T > 1."""
    if return_period <= 1.0:
        raise ValueError("return period must be greater than one")
    p = 1.0 - 1.0 / return_period
    return fit["location"] - fit["scale"] * math.log(-math.log(p))


def empirical_exceedance(values):
    """Sort descending; empirical exceedance prob = rank / (n + 1).

    Ties share the average rank. Returns a list of dicts sorted by
    descending value.
    """
    n = len(values)
    order = sorted(range(n), key=lambda i: values[i], reverse=True)
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
    rows = [
        {"value": values[idx], "rank": ranks[idx],
         "exceedance_probability": ranks[idx] / (n + 1)}
        for idx in order
    ]
    return rows
