"""Event-driven 2D rigid-disk impact simulator (translation + spin)."""

from .core import Disk, Simulation, SimConfig, SimError, run

__all__ = ["Disk", "Simulation", "SimConfig", "SimError", "run"]
