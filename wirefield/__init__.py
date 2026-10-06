"""Superposition of magnetic fields from closed polyline coils."""

from .biot import MU0_OVER_4PI, point_segment_distance, segment_field
from .model import Coil, CoilDataError, load_coils
from .probe import Probe, ProbeDataError, load_probes
from .scenario import Scenario, baseline_scenario, load_scenarios
from .solver import PointResult, evaluate

__all__ = [
    "MU0_OVER_4PI",
    "Coil",
    "CoilDataError",
    "PointResult",
    "Probe",
    "ProbeDataError",
    "Scenario",
    "baseline_scenario",
    "evaluate",
    "load_coils",
    "load_probes",
    "load_scenarios",
    "point_segment_distance",
    "segment_field",
]
