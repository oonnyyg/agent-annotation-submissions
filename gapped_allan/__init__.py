"""Gapped overlapping Allan deviation analysis (standard library only)."""

from .core import (
    Segment,
    ScaleResult,
    AnalysisResult,
    split_segments,
    overlapping_allan,
    analyze,
)

__all__ = [
    "Segment",
    "ScaleResult",
    "AnalysisResult",
    "split_segments",
    "overlapping_allan",
    "analyze",
]
