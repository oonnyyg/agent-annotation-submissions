"""Dry-friction slider motion under piecewise-constant forcing."""

from .core import Scenario, ForceSegment, simulate, ValidationError

__all__ = ["Scenario", "ForceSegment", "simulate", "ValidationError"]
