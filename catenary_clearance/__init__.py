"""Catenary clearance analysis for overhead cable spans (2D static self-weight)."""

from .solver import Catenary, CatenaryError, solve_catenary
from .clearance import clearance_against_terrain

__all__ = [
    "Catenary",
    "CatenaryError",
    "solve_catenary",
    "clearance_against_terrain",
]

__version__ = "0.1.0"
