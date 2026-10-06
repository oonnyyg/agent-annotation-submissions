"""Catenary clearance checker for overhead cable spans (2D, self-weight statics)."""

from .solver import CatenarySolution, SolveError
from .clearance import min_clearance

__all__ = [
    "CatenarySolution",
    "SolveError",
    "min_clearance",
]

version = "0.1.0"
