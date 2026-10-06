"""Planar four-bar linkage position/velocity audit tool."""

from .core import (
    AMBIGUOUS,
    OK,
    SINGULAR,
    UNREACHABLE,
    ConfigResult,
    Linkage,
    analyze,
)

__all__ = [
    "AMBIGUOUS",
    "OK",
    "SINGULAR",
    "UNREACHABLE",
    "ConfigResult",
    "Linkage",
    "analyze",
]
