"""Steady-state Gaussian plume core math (flat terrain, ground reflection)."""

from __future__ import annotations

import math
from dataclasses import dataclass


@dataclass(frozen=True)
class DispersionCoefficients:
    """Power-law dispersion widths: sigma = coeff * x**exponent (x in metres)."""

    lateral_coeff: float
    lateral_exponent: float
    vertical_coeff: float
    vertical_exponent: float

    def __post_init__(self) -> None:
        for name in (
            "lateral_coeff",
            "lateral_exponent",
            "vertical_coeff",
            "vertical_exponent",
        ):
            value = getattr(self, name)
            if not (isinstance(value, (int, float)) and math.isfinite(value) and value > 0):
                raise ValueError(f"dispersion {name} must be a positive finite number")

    def sigma_y(self, downwind_m: float) -> float:
        return self.lateral_coeff * downwind_m ** self.lateral_exponent

    def sigma_z(self, downwind_m: float) -> float:
        return self.vertical_coeff * downwind_m ** self.vertical_exponent


def wind_relative_coordinates(
    dx_east_m: float,
    dy_north_m: float,
    wind_from_deg: float,
) -> tuple[float, float]:
    """Convert east/north offsets to (downwind, crosswind) distances in metres.

    Wind direction is the direction the wind comes FROM, measured clockwise
    from true north (meteorological convention). The downwind axis points
    along where the wind blows TO.
    """
    theta = math.radians(wind_from_deg)
    # Unit vector the wind blows toward (opposite of where it comes from).
    to_east = -math.sin(theta)
    to_north = -math.cos(theta)
    downwind = dx_east_m * to_east + dy_north_m * to_north
    # Crosswind: perpendicular, sign irrelevant for Gaussian (y**2).
    crosswind = dx_east_m * to_north - dy_north_m * to_east
    return downwind, crosswind


def concentration(
    emission_rate: float,
    wind_speed: float,
    downwind_m: float,
    crosswind_m: float,
    source_height_m: float,
    receptor_height_m: float,
    coeffs: DispersionCoefficients,
    decay_rate: float = 0.0,
) -> float:
    """Steady-state Gaussian plume concentration with ground reflection.

    emission_rate: mass per time; wind_speed: m/s; decay_rate: 1/s.
    Returns mass per cubic metre. Downwind distance <= 0 gives zero.
    """
    if downwind_m <= 0.0:
        return 0.0
    sigma_y = coeffs.sigma_y(downwind_m)
    sigma_z = coeffs.sigma_z(downwind_m)
    lateral = math.exp(-0.5 * (crosswind_m / sigma_y) ** 2)
    z_sum = receptor_height_m - source_height_m
    z_ref = receptor_height_m + source_height_m
    vertical = math.exp(-0.5 * (z_sum / sigma_z) ** 2) + math.exp(
        -0.5 * (z_ref / sigma_z) ** 2
    )
    decay = math.exp(-decay_rate * downwind_m / wind_speed)
    return (
        emission_rate
        / (2.0 * math.pi * wind_speed * sigma_y * sigma_z)
        * lateral
        * vertical
        * decay
    )
