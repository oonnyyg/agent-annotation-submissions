"""Velocity model definition and validation.

All units are metres and seconds. A model is a stack of horizontal
layers; the last layer is an infinite half-space (thickness ignored).
Velocities must be positive and strictly increasing with depth.
"""

from __future__ import annotations

import math
from dataclasses import dataclass


class ModelError(ValueError):
    """Raised when a velocity model is physically invalid."""


@dataclass(frozen=True)
class Layer:
    """One horizontal layer.

    velocity: P-wave speed in m/s (must be positive).
    thickness: layer thickness in m; must be positive for every layer
        except the final half-space, where it is ignored and may be
        omitted (stored as math.inf).
    """

    velocity: float
    thickness: float = math.inf


class VelocityModel:
    """A validated stack of horizontal layers over a half-space."""

    def __init__(self, layers: list[Layer]):
        if not layers:
            raise ModelError("model must contain at least one layer")
        for index, layer in enumerate(layers):
            if not math.isfinite(layer.velocity) or layer.velocity <= 0.0:
                raise ModelError(
                    f"layer {index + 1}: velocity must be a positive "
                    f"finite number, got {layer.velocity!r}"
                )
            last = index == len(layers) - 1
            if last:
                if layer.thickness != math.inf and layer.thickness <= 0.0:
                    raise ModelError(
                        "half-space thickness, if given, must be positive"
                    )
            elif not math.isfinite(layer.thickness) or layer.thickness <= 0.0:
                raise ModelError(
                    f"layer {index + 1}: thickness must be a positive "
                    f"finite number, got {layer.thickness!r}"
                )
        for index in range(len(layers) - 1):
            if not layers[index].velocity < layers[index + 1].velocity:
                raise ModelError(
                    "velocities must strictly increase with depth: "
                    f"layer {index + 1} ({layers[index].velocity}) is not "
                    f"slower than layer {index + 2} "
                    f"({layers[index + 1].velocity})"
                )
        self.layers = tuple(layers)

    @property
    def half_space(self) -> Layer:
        return self.layers[-1]

    @property
    def interface_count(self) -> int:
        """Number of refracting interfaces (one per layer boundary)."""
        return len(self.layers) - 1

    @classmethod
    def from_dict(cls, data: dict) -> "VelocityModel":
        if not isinstance(data, dict) or "layers" not in data:
            raise ModelError("model JSON must be an object with a 'layers' list")
        raw_layers = data["layers"]
        if not isinstance(raw_layers, list):
            raise ModelError("'layers' must be a list")
        layers = []
        for index, raw in enumerate(raw_layers):
            if not isinstance(raw, dict) or "velocity" not in raw:
                raise ModelError(
                    f"layer {index + 1}: object with a 'velocity' is required"
                )
            try:
                velocity = float(raw["velocity"])
            except (TypeError, ValueError) as exc:
                raise ModelError(
                    f"layer {index + 1}: velocity is not a number"
                ) from exc
            if "thickness" in raw and raw["thickness"] is not None:
                try:
                    thickness = float(raw["thickness"])
                except (TypeError, ValueError) as exc:
                    raise ModelError(
                        f"layer {index + 1}: thickness is not a number"
                    ) from exc
            else:
                thickness = math.inf
            layers.append(Layer(velocity=velocity, thickness=thickness))
        return cls(layers)
