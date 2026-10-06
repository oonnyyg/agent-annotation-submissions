"""Welch cross-spectrum analysis for dual-channel synchronous signals."""

from .core import (
    Config,
    SegmentStats,
    SpectrumResult,
    compute_cross_spectrum,
    load_config,
    read_csv_channels,
)

__all__ = [
    "Config",
    "SegmentStats",
    "SpectrumResult",
    "compute_cross_spectrum",
    "load_config",
    "read_csv_channels",
]
