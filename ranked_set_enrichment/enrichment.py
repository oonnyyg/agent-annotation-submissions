"""Core enrichment statistics for a ranked feature list.

Implements a GSEA-style running-sum statistic with permutation-based
empirical p-values, using only the Python standard library. This is not a
re-implementation of the full GSEA software; it covers the specific
two-sided test described in the project documentation.
"""

from __future__ import annotations

import random
from dataclasses import dataclass


@dataclass(frozen=True)
class RankedFeature:
    """One entry of the ranked list, sorted by score descending."""

    identifier: str
    score: float


@dataclass(frozen=True)
class SetPreparation:
    """Result of filtering a raw feature set against the ranked list."""

    members: tuple[str, ...]
    duplicate_members: tuple[str, ...]
    missing_members: tuple[str, ...]
    computable: bool
    reason: str | None


def sort_ranked_list(features: list[RankedFeature]) -> list[RankedFeature]:
    """Sort by score descending; ties broken by identifier ascending."""
    return sorted(features, key=lambda f: (-f.score, f.identifier))


def prepare_set(
    raw_members: list[str], ranked_ids: list[str]
) -> SetPreparation:
    """Deduplicate members (first occurrence wins) and drop unknown ids."""
    seen: set[str] = set()
    members: list[str] = []
    duplicates: list[str] = []
    for member in raw_members:
        if member in seen:
            duplicates.append(member)
            continue
        seen.add(member)
        members.append(member)
    ranked_set = set(ranked_ids)
    present = [m for m in members if m in ranked_set]
    missing = [m for m in members if m not in ranked_set]
    computable = True
    reason = None
    if not present:
        computable = False
        reason = "empty_after_filtering"
    elif len(present) == len(ranked_ids):
        computable = False
        reason = "covers_entire_ranked_list"
    return SetPreparation(
        members=tuple(present),
        duplicate_members=tuple(duplicates),
        missing_members=tuple(missing),
        computable=computable,
        reason=reason,
    )


def running_enrichment(
    scores: list[float], hit_positions: set[int]
) -> tuple[list[float], float, int]:
    """Compute the cumulative walk along the ranked list.

    Hits add |score| normalized within the set (equal shares when all hit
    weights are zero); misses subtract 1 / (N - H). Returns the per-position
    walk values, the signed enrichment score (extremum of |walk|, positive
    on ties, earliest position on equal values) and its 0-based position.
    """
    n = len(scores)
    hits = len(hit_positions)
    if hits == 0 or hits == n:
        raise ValueError("set must be non-empty and smaller than the list")
    weights = {pos: abs(scores[pos]) for pos in hit_positions}
    total = sum(weights.values())
    if total == 0.0:
        increment = {pos: 1.0 / hits for pos in hit_positions}
    else:
        increment = {pos: w / total for pos, w in weights.items()}
    miss_step = 1.0 / (n - hits)
    walk: list[float] = []
    value = 0.0
    best_value = 0.0
    best_position = 0
    for pos in range(n):
        if pos in hit_positions:
            value += increment[pos]
        else:
            value -= miss_step
        walk.append(value)
        if abs(value) > abs(best_value) or (
            abs(value) == abs(best_value) and value > best_value
        ):
            best_value = value
            best_position = pos
    return walk, best_value, best_position


def permutation_pvalue(
    scores: list[float],
    hits: int,
    observed: float,
    permutations: int,
    seed: int,
) -> tuple[float, int]:
    """Two-sided empirical p-value from position permutations.

    Scores and positions stay fixed; each permutation draws `hits`
    positions uniformly without replacement. The p-value is
    (exceedances + 1) / (permutations + 1) where an exceedance is a
    permutation with |ES| >= |observed|.
    """
    if permutations < 1:
        raise ValueError("permutations must be >= 1")
    rng = random.Random(seed)
    n = len(scores)
    threshold = abs(observed)
    exceedances = 0
    for _ in range(permutations):
        positions = set(rng.sample(range(n), hits))
        _, es, _ = running_enrichment(scores, positions)
        if abs(es) >= threshold:
            exceedances += 1
    return (exceedances + 1) / (permutations + 1), exceedances


def bh_adjust(pvalues: list[float]) -> list[float]:
    """Benjamini-Hochberg adjusted p-values, input order preserved."""
    m = len(pvalues)
    order = sorted(range(m), key=lambda i: pvalues[i])
    adjusted = [0.0] * m
    running_min = 1.0
    for rank in range(m, 0, -1):
        idx = order[rank - 1]
        running_min = min(running_min, pvalues[idx] * m / rank)
        adjusted[idx] = min(running_min, 1.0)
    return adjusted


@dataclass(frozen=True)
class EnrichmentResult:
    """Full per-set analysis result."""

    name: str
    computable: bool
    reason: str | None
    effective_size: int
    enrichment_score: float | None
    extremum_position: int | None
    p_value: float | None
    exceedances: int | None
    walk: tuple[float, ...] | None
    missing_members: tuple[str, ...]
    duplicate_members: tuple[str, ...]


def compute_enrichment(
    name: str,
    raw_members: list[str],
    ranked: list[RankedFeature],
    permutations: int,
    seed: int,
) -> EnrichmentResult:
    """Run the full per-set analysis against the ranked list."""
    ranked_ids = [f.identifier for f in ranked]
    prep = prepare_set(raw_members, ranked_ids)
    base = dict(
        name=name,
        computable=prep.computable,
        reason=prep.reason,
        effective_size=len(prep.members),
        missing_members=prep.missing_members,
        duplicate_members=prep.duplicate_members,
    )
    if not prep.computable:
        return EnrichmentResult(
            enrichment_score=None,
            extremum_position=None,
            p_value=None,
            exceedances=None,
            walk=None,
            **base,
        )
    index_of = {identifier: pos for pos, identifier in enumerate(ranked_ids)}
    hit_positions = {index_of[m] for m in prep.members}
    scores = [f.score for f in ranked]
    walk, es, position = running_enrichment(scores, hit_positions)
    p_value, exceedances = permutation_pvalue(
        scores, len(hit_positions), es, permutations, seed
    )
    return EnrichmentResult(
        enrichment_score=es,
        extremum_position=position,
        p_value=p_value,
        exceedances=exceedances,
        walk=tuple(walk),
        **base,
    )
