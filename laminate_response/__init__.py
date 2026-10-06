"""Classical laminate theory small-deformation analysis with thermal loads."""

from .material import Material, Layer, Laminate
from .clt import analyze_laminate

__all__ = ["Material", "Layer", "Laminate", "analyze_laminate"]
