"""Travel-time paths: direct surface wave and critically refracted waves.

Every path is linear in offset x. A refracted path at interface i
exists only for x >= x_crit (the critical, or minimum emergence,
distance); below that offset the head wave does not exist and the
path is reported as unreachable rather than extrapolated.
"""

from __future__ import annotations

import math
from dataclasses import dataclass

from .model import VelocityModel

# Relative/absolute tolerance for declaring equal travel times.
TIME_TOLERANCE = 1e-9


@dataclass(frozen=True)
class Segment:
    """One straight leg of a path.

    layer: 1-based index of the layer the leg travels through
        (the head-wave leg travels along the top of layer
        ``interface + 1`` and is reported with that layer index).
    role: "down", "up", "headwave", or "direct".
    """

    layer: int
    role: str
    length: float
    horizontal: float
    time: float


@dataclass(frozen=True)
class Path:
    """A candidate propagation path, linear in offset."""

    kind: str  # "direct" or "refracted"
    interface: int | None  # 1-based refracting interface, None for direct
    slope: float  # dt/dx = 1 / apparent velocity
    intercept: float  # time at x = 0 (0 for the direct wave)
    min_offset: float  # critical distance; 0 for the direct wave
    segments_at: object  # callable(x) -> list[Segment]

    def reachable(self, x: float, tol: float = TIME_TOLERANCE) -> bool:
        return x >= self.min_offset - tol * max(1.0, abs(self.min_offset))

    def time(self, x: float) -> float:
        return self.slope * x + self.intercept

    def segments(self, x: float) -> list[Segment]:
        return self.segments_at(x)


def direct_path(model: VelocityModel) -> Path:
    velocity = model.layers[0].velocity

    def segments(x: float) -> list[Segment]:
        return [Segment(1, "direct", x, x, x / velocity)]

    return Path("direct", None, 1.0 / velocity, 0.0, 0.0, segments)


def refracted_path(model: VelocityModel, interface: int) -> Path:
    """Head wave refracted at ``interface`` (1-based, bottom of that layer).

    The head wave travels along the top of layer ``interface + 1``.
    """
    layers = model.layers
    v_head = layers[interface].velocity  # layer below the interface
    intercept = 0.0
    min_offset = 0.0
    cosines: list[float] = []
    for j in range(interface):
        ratio = layers[j].velocity / v_head
        cos_j = math.sqrt(1.0 - ratio * ratio)
        cosines.append(cos_j)
        thickness = layers[j].thickness
        intercept += 2.0 * thickness * cos_j / layers[j].velocity
        min_offset += 2.0 * thickness * (ratio / cos_j)

    def segments(x: float) -> list[Segment]:
        result: list[Segment] = []
        for j in range(interface):
            thickness = layers[j].thickness
            cos_j = cosines[j]
            leg_length = thickness / cos_j
            leg_horizontal = thickness * math.sqrt(1.0 - cos_j * cos_j) / cos_j
            leg_time = leg_length / layers[j].velocity
            result.append(
                Segment(j + 1, "down", leg_length, leg_horizontal, leg_time)
            )
        head_horizontal = x - min_offset
        result.append(
            Segment(
                interface + 1,
                "headwave",
                head_horizontal,
                head_horizontal,
                head_horizontal / v_head,
            )
        )
        for j in reversed(range(interface)):
            thickness = layers[j].thickness
            cos_j = cosines[j]
            leg_length = thickness / cos_j
            leg_horizontal = thickness * math.sqrt(1.0 - cos_j * cos_j) / cos_j
            leg_time = leg_length / layers[j].velocity
            result.append(
                Segment(j + 1, "up", leg_length, leg_horizontal, leg_time)
            )
        return result

    return Path(
        "refracted", interface, 1.0 / v_head, intercept, min_offset, segments
    )


def refracted_paths(model: VelocityModel) -> list[Path]:
    return [
        refracted_path(model, interface)
        for interface in range(1, model.interface_count + 1)
    ]


def all_paths(model: VelocityModel) -> list[Path]:
    return [direct_path(model)] + refracted_paths(model)


def first_arrivals(
    model: VelocityModel, x: float, tol: float = TIME_TOLERANCE
) -> tuple[float, list[Path]]:
    """Earliest time and all paths tied within ``tol`` at offset ``x``."""
    if x < 0.0:
        raise ValueError("offset must be non-negative")
    candidates = [p for p in all_paths(model) if p.reachable(x, tol)]
    best = min(p.time(x) for p in candidates)
    scale = max(1.0, abs(best))
    tied = [p for p in candidates if abs(p.time(x) - best) <= tol * scale]
    return best, tied
