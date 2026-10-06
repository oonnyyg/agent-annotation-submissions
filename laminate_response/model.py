"""Input data model: materials, plies, laminate and load cases."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass


class InputError(ValueError):
    """Raised when the input document fails validation."""


@dataclass(frozen=True)
class Material:
    """Plane-stress orthotropic material in its principal axes (1, 2)."""

    name: str
    e1: float
    e2: float
    nu12: float
    g12: float
    alpha1: float
    alpha2: float

    @property
    def nu21(self) -> float:
        return self.nu12 * self.e2 / self.e1

    def validate(self) -> None:
        for field in ("e1", "e2", "g12"):
            if getattr(self, field) <= 0.0:
                raise InputError(
                    f"material {self.name!r}: {field} must be positive"
                )
        # Positive definiteness of the plane-stress stiffness requires
        # 1 - nu12*nu21 > 0 together with positive moduli.
        if self.nu12 * self.nu21 >= 1.0:
            raise InputError(
                f"material {self.name!r}: stiffness not positive definite "
                f"(nu12*nu21 = {self.nu12 * self.nu21:.6g} >= 1)"
            )

    def q_matrix(self) -> list[list[float]]:
        """Reduced stiffness in material axes, engineering shear strain."""
        self.validate()
        denom = 1.0 - self.nu12 * self.nu21
        q11 = self.e1 / denom
        q12 = self.nu12 * self.e2 / denom
        q22 = self.e2 / denom
        return [[q11, q12, 0.0], [q12, q22, 0.0], [0.0, 0.0, self.g12]]


@dataclass(frozen=True)
class Ply:
    material: str
    angle_deg: float
    thickness: float

    def validate(self) -> None:
        if self.thickness <= 0.0:
            raise InputError(
                f"ply (material {self.material!r}): thickness must be positive"
            )
        if not math.isfinite(self.angle_deg):
            raise InputError(
                f"ply (material {self.material!r}): angle must be finite"
            )


@dataclass(frozen=True)
class Laminate:
    """Stacking ordered from bottom (most negative z) to top."""

    plies: tuple[Ply, ...]

    @property
    def total_thickness(self) -> float:
        return sum(p.thickness for p in self.plies)

    def interfaces(self) -> list[float]:
        """z coordinates of ply boundaries, midplane at z = 0."""
        z = -self.total_thickness / 2.0
        coords = [z]
        for ply in self.plies:
            z += ply.thickness
            coords.append(z)
        return coords


@dataclass(frozen=True)
class LoadCase:
    name: str
    nx: float
    ny: float
    nxy: float
    mx: float
    my: float
    mxy: float
    delta_t: float


def _require(mapping: dict, key: str, where: str):
    if key not in mapping:
        raise InputError(f"{where}: missing required key {key!r}")
    return mapping[key]


def load_document(path: str) -> tuple[dict[str, Material], Laminate, list[LoadCase]]:
    with open(path, "r", encoding="utf-8") as fh:
        doc = json.load(fh)

    materials: dict[str, Material] = {}
    for entry in _require(doc, "materials", "document"):
        name = _require(entry, "name", "material")
        mat = Material(
            name=name,
            e1=float(_require(entry, "E1", f"material {name!r}")),
            e2=float(_require(entry, "E2", f"material {name!r}")),
            nu12=float(_require(entry, "nu12", f"material {name!r}")),
            g12=float(_require(entry, "G12", f"material {name!r}")),
            alpha1=float(_require(entry, "alpha1", f"material {name!r}")),
            alpha2=float(_require(entry, "alpha2", f"material {name!r}")),
        )
        mat.validate()
        materials[name] = mat

    plies = []
    for i, entry in enumerate(_require(doc, "layup", "document")):
        ply = Ply(
            material=_require(entry, "material", f"layup[{i}]"),
            angle_deg=float(_require(entry, "angle_deg", f"layup[{i}]")),
            thickness=float(_require(entry, "thickness", f"layup[{i}]")),
        )
        ply.validate()
        if ply.material not in materials:
            raise InputError(f"layup[{i}]: unknown material {ply.material!r}")
        plies.append(ply)
    if not plies:
        raise InputError("layup must contain at least one ply")
    laminate = Laminate(tuple(plies))

    cases = []
    for entry in _require(doc, "cases", "document"):
        name = entry.get("name", f"case_{len(cases)}")
        loads = entry.get("loads", {})
        cases.append(
            LoadCase(
                name=name,
                nx=float(loads.get("Nx", 0.0)),
                ny=float(loads.get("Ny", 0.0)),
                nxy=float(loads.get("Nxy", 0.0)),
                mx=float(loads.get("Mx", 0.0)),
                my=float(loads.get("My", 0.0)),
                mxy=float(loads.get("Mxy", 0.0)),
                delta_t=float(entry.get("delta_T", 0.0)),
            )
        )
    if not cases:
        raise InputError("document must define at least one case")
    return materials, laminate, cases
