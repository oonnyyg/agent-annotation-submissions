"""Residuals between modelled first arrivals and observed picks.

This is a forward comparison only: no parameter inversion is done.
"""
from __future__ import annotations

import math
from dataclasses import dataclass

from .model import VelocityModel
from .traveltime import first_arrivals


@dataclass(frozen=True)
class Residual:
    offset: float
    observed: float
    modelled: float
    residual: float  # observed - modelled


@dataclass(frozen=True)
class ResidualReport:
    residuals: tuple[Residual, ...]
    rms: float
    mean: float
    max_abs: float


def residual_report(
    model: VelocityModel, picks: list[tuple[float, float]]
) -> ResidualReport:
    """Compare observed (offset, time) picks with model first arrivals."""
    residuals = []
    for offset, observed in picks:
        if offset < 0.0:
            raise ValueError("observed offsets must be non-negative")
        modelled, _ = first_arrivals(model, offset)
        residuals.append(
            Residual(offset, observed, modelled, observed - modelled)
        )
    if not residuals:
        return ResidualReport((), math.nan, math.nan, math.nan)
    squares = sum(r.residual**2 for r in residuals)
    rms = math.sqrt(squares / len(residuals))
    mean = sum(r.residual for r in residuals) / len(residuals)
    max_abs = max(abs(r.residual) for r in residuals)
    return ResidualReport(tuple(residuals), rms, mean, max_abs)
