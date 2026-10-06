"""Layered velocity model definition and validation."""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Layer:
    """One horizontal layer. The last layer is an infinite half-space."""

    velocity: float  # m/s, must be positive
    thickness: float | None  # m, positive; None only for the final half-space


class VelocityModel:
    """Horizontal stack of layers with strictly increasing velocity."""

    def __init__(self, layers: list[Layer]):
        if not layers:
            raise ValueError("model must contain at least one layer")
        for i, layer in enumerate(layers):
            if not (layer.velocity > 0.0):
                raise ValueError(
                    f"layer {i + 1}: velocity must be positive, got {layer.velocity}"
                )
            if i < len(layers) - 1:
                if layer.thickness is None or not (layer.thickness > 0.0):
                    raise ValueError(
                        f"layer {i + 1}: thickness must be positive, "
                        f"got {layer.thickness}"
                    )
            elif layer.thickness is not None:
                raise ValueError("final half-space must not have a thickness")
        for i in range(1, len(layers)):
            if not layers[i].velocity > layers[i - 1].velocity:
                raise ValueError(
                    "velocities must strictly increase with depth: "
                    f"layer {i} ({layers[i - 1].velocity}) vs "
                    f"layer {i + 1} ({layers[i].velocity})"
                )
        self.layers = list(layers)

    @property
    def velocities(self) -> list[float]:
        return [layer.velocity for layer in self.layers]

    @property
    def thicknesses(self) -> list[float]:
        return [layer.thickness for layer in self.layers[:-1]]

    @classmethod
    def from_dict(cls, data: dict) -> "VelocityModel":
        raw = data.get("layers")
        if not isinstance(raw, list):
            raise ValueError("model JSON must contain a 'layers' list")
        layers = []
        for i, entry in enumerate(raw):
            try:
                velocity = float(entry["velocity"])
            except (KeyError, TypeError, ValueError) as exc:
                raise ValueError(f"layer {i + 1}: invalid velocity") from exc
            thickness = entry.get("thickness")
            if thickness is not None:
                try:
                    thickness = float(thickness)
                except (TypeError, ValueError) as exc:
                    raise ValueError(f"layer {i + 1}: invalid thickness") from exc
            layers.append(Layer(velocity=velocity, thickness=thickness))
        return cls(layers)
