"""Shaded PV string calculator (simplified piecewise-linear model)."""

from .model import Module
from .solver import analyze_condition, sweep

__all__ = ["Module", "analyze_condition", "sweep"]
