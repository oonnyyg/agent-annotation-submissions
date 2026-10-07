"""Gapped overlapping Allan deviation analysis (standard library only)."""

from .core import (
    AnalysisError,
    Segment,
    TauResult,
    build_segments,
    overlapping_allan,
    detrend_ols,
)

__all__ = [
    "AnalysisError",
    "Segment",
    "TauResult",
    "build_segments",
    "overlapping_allan",
    "detrend_ols",
]
