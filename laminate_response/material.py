"""Material and layup definitions with validation."""

from __future__ import annotations

import math
from dataclasses import dataclass


class InputError(ValueError):
    """Raised for invalid user input."""


@dataclass(frozen=True)
class Material:
    """Plane-stress orthotropic lamina material.

    e1, e2: Young's moduli along material axes 1 and 2.
    nu12: major Poisson's ratio.
    g12: in-plane shear modulus.
    alpha1, alpha2: thermal expansion coefficients along axes 1 and 2.
    """

    name: str
    e1: float
    e2: float
    nu12: float
    g12: float
    alpha1: float
    alpha2: float

    def validate(self) -> None:
        if self.e1 <= 0.0 or self.e2 <= 0.0 or self.g12 <= 0.0:
            raise InputError(
                f"material {self.name!r}: moduli e1, e2, g12 must be positive"
            )
        nu21 = self.nu12 * self.e2 / self.e1
        denom = 1.0 - self.nu12 * nu21
        if denom <= 0.0:
            raise InputError(
                f"material {self.name!r}: stiffness not positive definite "
                f"(1 - nu12*nu21 = {denom:g})"
            )

    @property
    def nu21(self) -> float:
        return self.nu12 * self.e2 / self.e1

    def q_matrix(self) -> list[list[float]]:
        """Reduced plane-stress stiffness in material axes."""
        self.validate()
        d = 1.0 - self.nu12 * self.nu21
        return [
            [self.e1 / d, self.nu12 * self.e2 / d, 0.0],
            [self.nu12 * self.e2 / d, self.e2 / d, 0.0],
            [0.0, 0.0, self.g12],
        ]

    def alpha_vector(self) -> list[float]:
        """Thermal expansion vector in material axes (engineering shear)."""
        return [self.alpha1, self.alpha2, 0.0]


@dataclass(frozen=True)
class Layer:
    """A single ply: material, thickness, and orientation angle in degrees.

    Angle is measured counterclockwise from the global x axis to the
    material 1-axis.
    """

    material: Material
    thickness: float
    angle_deg: float

    def validate(self) -> None:
        if self.thickness <= 0.0:
            raise InputError("layer thickness must be positive")
        if not math.isfinite(self.angle_deg):
            raise InputError("layer angle must be finite")
        self.material.validate()


@dataclass(frozen=True)
class Laminate:
    """Stack of layers ordered from bottom (most negative z) to top."""

    layers: tuple[Layer, ...]

    def validate(self) -> None:
        if not self.layers:
            raise InputError("laminate must contain at least one layer")
        for layer in self.layers:
            layer.validate()

    @property
    def total_thickness(self) -> float:
        return sum(layer.thickness for layer in self.layers)

    def interfaces(self) -> list[float]:
        """z coordinates of layer interfaces, midplane at z=0."""
        total = self.total_thickness
        zs = [-total / 2.0]
        for layer in self.layers:
            zs.append(zs[-1] + layer.thickness)
        return zs
