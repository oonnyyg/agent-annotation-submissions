"""Monte Carlo simulation under complete spatial randomness (CSR).

Each simulated configuration places the same number of points
independently and uniformly at random inside the same rectangle, using a
fixed random seed for reproducibility.
"""

from __future__ import annotations

import random

from .core import k_estimate, l_from_k, ordered_pair_distances


def simulate_l_curves(window, radii, n_points, simulations, seed):
    """Return the list of simulated L curves (one list of L values each)."""
    rng = random.Random(seed)
    curves = []
    for _ in range(simulations):
        pts = [
            (rng.random() * window.width, rng.random() * window.height)
            for _ in range(n_points)
        ]
        pairs = ordered_pair_distances(pts)
        _, k_vals = k_estimate(window, pairs, radii, correct=True)
        curves.append(l_from_k(k_vals))
    return curves


def deviation_statistic(l_values, radii):
    """Max absolute deviation of L(r) - r over the whole radius grid."""
    return max(abs(l - r) for l, r in zip(l_values, radii))


def envelope(curves):
    """Pointwise min/max across simulated curves.

    This is a pointwise envelope only; it is NOT a simultaneous
    confidence band.
    """
    mins = [min(col) for col in zip(*curves)]
    maxs = [max(col) for col in zip(*curves)]
    return mins, maxs


def upper_tail_pvalue(observed, simulated_stats):
    """Plus-one corrected Monte Carlo upper-tail probability.

    Ties count towards the tail: p = (1 + #{s >= obs}) / (1 + m).
    """
    exceed = sum(1 for s in simulated_stats if s >= observed)
    return (1 + exceed) / (1 + len(simulated_stats))
