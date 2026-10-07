"""Orchestration: run all feature sets against the ranked list."""

from __future__ import annotations

from .core import (
    EnrichmentResult,
    FeatureScore,
    FeatureSet,
    compute_running_sum,
    enrichment_score,
)
from .stats import benjamini_hochberg, empirical_p_value, permuted_scores


def analyze(
    ranked: list[FeatureScore],
    feature_sets: list[FeatureSet],
    permutations: int,
    seed: int,
) -> list[EnrichmentResult]:
    """Run enrichment for every set and apply BH correction across valid sets."""
    index_of = {record.feature_id: pos for pos, record in enumerate(ranked)}
    total = len(ranked)
    results: list[EnrichmentResult] = []
    for feature_set in feature_sets:
        unique_members = list(dict.fromkeys(feature_set.members))
        hit_positions = sorted(
            index_of[m] for m in unique_members if m in index_of
        )
        ignored = [m for m in unique_members if m not in index_of]
        result = EnrichmentResult(
            name=feature_set.name,
            status="ok",
            member_count=len(hit_positions),
            ignored_members=ignored,
        )
        if not hit_positions:
            result.status = "not_computable"
            result.reason = "no members present in the ranked list after filtering"
        elif len(hit_positions) == total:
            result.status = "not_computable"
            result.reason = "set covers the entire ranked list"
        else:
            walk = compute_running_sum(ranked, set(hit_positions))
            score, peak = enrichment_score(walk)
            result.hit_positions = hit_positions
            result.enrichment_score = score
            result.peak_position = peak
            result.running_sum = walk
            null = permuted_scores(ranked, len(hit_positions), permutations, seed)
            result.p_value = empirical_p_value(score, null)
        results.append(result)

    valid = [r for r in results if r.status == "ok"]
    q_values = benjamini_hochberg([r.p_value for r in valid])
    for result, q_value in zip(valid, q_values):
        result.q_value = q_value
    return results
