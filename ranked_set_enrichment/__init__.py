"""Offline ranked-set enrichment analysis using only the standard library."""

from .core import (
    EnrichmentResult,
    FeatureScore,
    FeatureSet,
    compute_running_sum,
    enrichment_score,
    load_feature_scores,
    load_feature_sets,
)

__all__ = [
    "EnrichmentResult",
    "FeatureScore",
    "FeatureSet",
    "compute_running_sum",
    "enrichment_score",
    "load_feature_scores",
    "load_feature_sets",
]

__version__ = "0.1.0"
