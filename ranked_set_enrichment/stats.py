"""Permutation null distribution and multiple-testing correction."""

from __future__ import annotations

import random

from .core import FeatureScore, compute_running_sum, enrichment_score


def permuted_scores(
    ranked: list[FeatureScore],
    member_count: int,
    permutations: int,
    seed: int,
) -> list[float]:
    """Null enrichment scores from uniform sampling without replacement.

    Scores and positions stay fixed; each permutation draws ``member_count``
    distinct positions uniformly at random. The RNG is seeded per call so a
    set's null distribution depends only on its size, not on input order.
    """
    rng = random.Random(seed)
    total = len(ranked)
    null: list[float] = []
    for _ in range(permutations):
        positions = set(rng.sample(range(total), member_count))
        walk = compute_running_sum(ranked, positions)
        score, _ = enrichment_score(walk)
        null.append(score)
    return null


def empirical_p_value(observed: float, null: list[float]) -> float:
    """Two-sided empirical p-value with +1 smoothing on numerator/denominator."""
    extreme = sum(1 for value in null if abs(value) >= abs(observed))
    return (extreme + 1) / (len(null) + 1)


def benjamini_hochberg(p_values: list[float]) -> list[float]:
    """Benjamini-Hochberg adjusted q-values, aligned with the input order."""
    count = len(p_values)
    order = sorted(range(count), key=lambda i: p_values[i])
    adjusted = [0.0] * count
    running_min = 1.0
    for rank_from_end, index in enumerate(reversed(order), start=1):
        rank = count - rank_from_end + 1
        running_min = min(running_min, p_values[index] * count / rank)
        adjusted[index] = min(running_min, 1.0)
    return adjusted
