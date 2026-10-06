"""Dry-friction stick-slip motion of a horizontal block."""

from .model import Scenario, ForceSegment, load_scenarios
from .simulate import simulate, sample_trajectory

__all__ = [
    "Scenario",
    "ForceSegment",
    "load_scenarios",
    "simulate",
    "sample_trajectory",
]
