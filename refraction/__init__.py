"""First-arrival refraction analysis for horizontal layered models."""

from .model import Layer, VelocityModel
from .paths import Path, compute_paths
from .analysis import first_arrivals, dominance_intervals

__all__ = [
    "Layer",
    "VelocityModel",
    "Path",
    "compute_paths",
    "first_arrivals",
    "dominance_intervals",
]
