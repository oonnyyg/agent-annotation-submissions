"""Analytic projection of a uniform disk, for synthetic validation data."""

from __future__ import annotations

import csv
import math


def disk_projection(
    angle_rad: float,
    positions: list[float],
    radius: float,
    attenuation: float,
    center: tuple[float, float] = (0.0, 0.0),
) -> list[float]:
    """Line-integral projection of a uniform disk.

    The disk has ``attenuation`` inside ``radius`` around ``center`` and
    zero outside.  For detector coordinate ``t`` at angle ``theta`` the
    chord length through the disk is ``2 * sqrt(R^2 - d^2)`` where ``d``
    is the distance from the ray to the disk centre.
    """
    x0, y0 = center
    t_center = x0 * math.cos(angle_rad) + y0 * math.sin(angle_rad)
    readings = []
    for t in positions:
        d = t - t_center
        if abs(d) >= radius:
            readings.append(0.0)
        else:
            readings.append(2.0 * attenuation * math.sqrt(radius * radius - d * d))
    return readings


def write_sinogram(
    path: str,
    angles_deg: list[float],
    positions: list[float],
    radius: float,
    attenuation: float,
    center: tuple[float, float] = (0.0, 0.0),
) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        for a in angles_deg:
            readings = disk_projection(
                math.radians(a), positions, radius, attenuation, center
            )
            writer.writerow([f"{a:.12g}"] + [f"{v:.12g}" for v in readings])
