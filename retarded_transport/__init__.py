"""Retarded 1-D solute transport with explicit finite volumes (stdlib only)."""

from .solver import simulate, TransportError

__all__ = ["simulate", "TransportError"]
