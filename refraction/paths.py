"""Ray path construction and travel-time computation.

All quantities are in metres and seconds. A refracted path along the top
of layer k (1-based) exists only for offsets >= its critical distance;
shorter offsets are marked unreachable rather than extrapolated.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .model import VelocityModel


@dataclass(frozen=True)
class LegTemplate:
    """A path leg whose length may scale with source-receiver offset."""

    layer: int  # 1-based layer index
    vertical: float  # m, fixed
    horizontal_fixed: float  # m, fixed part
    horizontal_coef: float  # multiplied by offset
    time_fixed: float  # s, fixed part
    time_coef: float  # multiplied by offset


@dataclass(frozen=True)
class Segment:
    """A concrete leg of a path at a given offset."""

    layer: int
    horizontal: float  # m
    vertical: float  # m
    time: float  # s


@dataclass
class Path:
    """A candidate path: direct wave or refraction along one interface."""

    kind: str  # "direct" or "refracted"
    refractor: int | None  # 1-based refracting layer, None for direct
    legs: list[LegTemplate] = field(default_factory=list)
    critical_distance: float = 0.0  # m, minimum offset for refracted paths

    def segments_at(self, offset: float) -> list[Segment]:
        return [
            Segment(
                layer=leg.layer,
                horizontal=leg.horizontal_fixed + leg.horizontal_coef * offset,
                vertical=leg.vertical,
                time=leg.time_fixed + leg.time_coef * offset,
            )
            for leg in self.legs
        ]

    def travel_time(self, offset: float) -> float:
        return sum(seg.time for seg in self.segments_at(offset))

    def reachable(self, offset: float, tol: float = 1e-9) -> bool:
        return offset >= self.critical_distance - tol


def _direct_path(model: VelocityModel) -> Path:
    v1 = model.velocities[0]
    leg = LegTemplate(
        layer=1, vertical=0.0,
        horizontal_fixed=0.0, horizontal_coef=1.0,
        time_fixed=0.0, time_coef=1.0 / v1,
    )
    return Path(kind="direct", refractor=None, legs=[leg])


def _refracted_path(model: VelocityModel, k: int) -> Path:
    """Head wave refracted along the top of layer k (1-based, k >= 2)."""
    velocities = model.velocities
    thicknesses = model.thicknesses
    vk = velocities[k - 1]
    legs: list[LegTemplate] = []
    critical = 0.0
    for i in range(k - 1):
        vi = velocities[i]
        hi = thicknesses[i]
        sin_i = vi / vk
        cos_i = math.sqrt(1.0 - sin_i * sin_i)
        tan_i = sin_i / cos_i
        # Down-going and up-going legs through layer i.
        for _ in range(2):
            legs.append(
                LegTemplate(
                    layer=i + 1, vertical=hi,
                    horizontal_fixed=hi * tan_i, horizontal_coef=0.0,
                    time_fixed=hi / (vi * cos_i), time_coef=0.0,
                )
            )
        critical += 2.0 * hi * tan_i
    # Horizontal leg along the refractor top: offset minus the fixed
    # horizontal span of the up/down legs (i.e. the critical distance).
    legs.append(
        LegTemplate(
            layer=k, vertical=0.0,
            horizontal_fixed=-critical, horizontal_coef=1.0,
            time_fixed=-critical / vk, time_coef=1.0 / vk,
        )
    )
    return Path(kind="refracted", refractor=k, legs=legs, critical_distance=critical)


def compute_paths(model: VelocityModel) -> list[Path]:
    """Direct wave plus one refracted wave per interface below layer 1."""
    paths = [_direct_path(model)]
    for k in range(2, len(model.layers) + 1):
        paths.append(_refracted_path(model, k))
    return paths
