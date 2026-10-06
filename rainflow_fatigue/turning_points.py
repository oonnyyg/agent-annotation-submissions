"""Extract turning points (peaks/valleys) from a stress history.

Rules:
- Consecutive equal stresses keep only the last point.
- Keep the first/last points and direction-reversal points.
- A constant record yields no cycles.
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Point:
    """One sample of the stress history."""

    time: float
    stress: float


def extract_turning_points(points: list[Point]) -> list[Point]:
    """Return the turning-point sequence, preserving original times."""
    if not points:
        return []

    deduped: list[Point] = [points[0]]
    for point in points[1:]:
        if point.stress == deduped[-1].stress:
            deduped[-1] = point
        else:
            deduped.append(point)

    if len(deduped) <= 2:
        return deduped

    turning: list[Point] = [deduped[0]]
    for index in range(1, len(deduped) - 1):
        prev_diff = deduped[index].stress - deduped[index - 1].stress
        next_diff = deduped[index + 1].stress - deduped[index].stress
        if prev_diff * next_diff < 0:
            turning.append(deduped[index])
    turning.append(deduped[-1])
    return turning
