"""Configuration loading and validation (SI units: m, kg, s)."""
from __future__ import annotations

import json
import math
from dataclasses import dataclass


class ConfigError(ValueError):
    """Raised when the input configuration is invalid."""


def _positive_finite(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{name} must be a number, got {value!r}")
    result = float(value)
    if not math.isfinite(result) or result <= 0.0:
        raise ConfigError(f"{name} must be positive and finite, got {result!r}")
    return result


@dataclass(frozen=True)
class ColumnConfig:
    depth: float
    area: float


@dataclass(frozen=True)
class FluidConfig:
    density: float
    viscosity: float


@dataclass(frozen=True)
class ParticleClass:
    name: str
    diameter: float
    density: float
    mass: float


@dataclass(frozen=True)
class Config:
    column: ColumnConfig
    fluid: FluidConfig
    gravity: float
    classes: tuple


def load_config(path):
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    return parse_config(data)


def parse_config(data):
    if not isinstance(data, dict):
        raise ConfigError("top-level JSON value must be an object")
    try:
        column_data = data["column"]
        fluid_data = data["fluid"]
        gravity = data["gravity"]
        classes_data = data["classes"]
    except (KeyError, TypeError) as exc:
        raise ConfigError(f"missing required key: {exc}") from exc

    column = ColumnConfig(
        depth=_positive_finite(column_data.get("depth"), "column.depth"),
        area=_positive_finite(column_data.get("area"), "column.area"),
    )
    fluid = FluidConfig(
        density=_positive_finite(fluid_data.get("density"), "fluid.density"),
        viscosity=_positive_finite(fluid_data.get("viscosity"), "fluid.viscosity"),
    )
    gravity = _positive_finite(gravity, "gravity")

    if not isinstance(classes_data, list) or not classes_data:
        raise ConfigError("classes must be a non-empty list")

    classes = []
    for index, item in enumerate(classes_data):
        label = item.get("name") or f"class_{index}"
        diameter = _positive_finite(item.get("diameter"), f"classes[{index}].diameter")
        density = _positive_finite(item.get("density"), f"classes[{index}].density")
        mass = _positive_finite(item.get("mass"), f"classes[{index}].mass")
        if density <= fluid.density:
            raise ConfigError(
                f"classes[{index}].density ({density}) must exceed fluid density "
                f"({fluid.density})"
            )
        classes.append(
            ParticleClass(name=str(label), diameter=diameter, density=density, mass=mass)
        )

    return Config(column=column, fluid=fluid, gravity=gravity, classes=tuple(classes))
