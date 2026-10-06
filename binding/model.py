"""Data model and validation. All concentrations are nM and must be finite."""

from __future__ import annotations

import math
from dataclasses import dataclass


def _require_finite(value: float, what: str) -> float:
    v = float(value)
    if not math.isfinite(v):
        raise ValueError(f"{what} must be finite, got {value!r}")
    return v


@dataclass(frozen=True)
class SiteClass:
    """One independent class of binding sites (1:1 binding, mutually exclusive)."""

    name: str
    total: float  # total site concentration, nM, > 0
    kd_a: float   # dissociation constant for ligand A, nM, > 0
    kd_b: float   # dissociation constant for ligand B, nM, > 0

    def __post_init__(self) -> None:
        if not self.name:
            raise ValueError("site class name must be non-empty")
        for field in ("total", "kd_a", "kd_b"):
            v = _require_finite(getattr(self, field), f"{self.name}.{field}")
            if v <= 0.0:
                raise ValueError(f"{self.name}.{field} must be positive, got {v}")


@dataclass(frozen=True)
class Experiment:
    """Total (analytical) concentrations of both ligands, nM, >= 0."""

    total_a: float
    total_b: float

    def __post_init__(self) -> None:
        for field in ("total_a", "total_b"):
            v = _require_finite(getattr(self, field), field)
            if v < 0.0:
                raise ValueError(f"{field} must be non-negative, got {v}")
