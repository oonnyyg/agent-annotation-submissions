"""Steady-state Gaussian plume with ground reflection (flat terrain)."""

from __future__ import annotations

import math


def wind_components(dx: float, dy: float, wind_from_deg: float) -> tuple[float, float]:
    """Project an east/north offset onto downwind/crosswind axes.

    ``wind_from_deg`` is the direction the wind comes FROM, measured
    clockwise from true north (meteorological convention).
    Returns (downwind, crosswind) distances in metres. The crosswind sign
    is arbitrary since concentrations depend on its square.
    """
    theta = math.radians(wind_from_deg)
    # Unit vector pointing downwind (where the plume travels to).
    ux = -math.sin(theta)
    uy = -math.cos(theta)
    downwind = dx * ux + dy * uy
    crosswind = dx * uy - dy * ux
    return downwind, crosswind


def concentration(
    emission_rate: float,
    wind_speed: float,
    source_height: float,
    receptor_height: float,
    downwind: float,
    crosswind: float,
    sigma_y_coeff: float,
    sigma_y_exp: float,
    sigma_z_coeff: float,
    sigma_z_exp: float,
    decay_rate: float = 0.0,
) -> float:
    """Steady-state Gaussian plume concentration (mass/volume).

    emission_rate: mass/time; wind_speed: m/s; heights and distances: m;
    decay_rate: 1/s (first-order, applied to downwind travel time x/u).
    Returns 0.0 when the receptor is not downwind of the source.
    """
    if downwind <= 0.0:
        return 0.0
    sigma_y = sigma_y_coeff * downwind**sigma_y_exp
    sigma_z = sigma_z_coeff * downwind**sigma_z_exp
    lateral = math.exp(-(crosswind**2) / (2.0 * sigma_y**2))
    dz_near = receptor_height - source_height
    dz_far = receptor_height + source_height
    vertical = math.exp(-(dz_near**2) / (2.0 * sigma_z**2)) + math.exp(
        -(dz_far**2) / (2.0 * sigma_z**2)
    )
    decay = math.exp(-decay_rate * downwind / wind_speed)
    return (
        emission_rate
        / (2.0 * math.pi * wind_speed * sigma_y * sigma_z)
        * lateral
        * vertical
        * decay
    )
