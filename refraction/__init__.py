"""First-arrival travel-time analysis for horizontal layered media."""

from .model import Layer, VelocityModel, ModelError
from .traveltime import (
    Path,
    direct_path,
    refracted_path,
    refracted_paths,
    first_arrivals,
)
from .branches import BranchInterval, branch_intervals
from .observed import ResidualReport, residual_report

__all__ = [
    "Layer",
    "VelocityModel",
    "ModelError",
    "Path",
    "direct_path",
    "refracted_path",
    "refracted_paths",
    "first_arrivals",
    "BranchInterval",
    "branch_intervals",
    "ResidualReport",
    "residual_report",
]
