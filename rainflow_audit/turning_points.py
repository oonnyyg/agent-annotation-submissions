"""CSV loading and turning-point extraction."""

from __future__ import annotations

import csv
import math
from dataclasses import dataclass


@dataclass(frozen=True)
class Point:
    time: float
    stress: float


def load_csv(path: str) -> list[Point]:
    """Load a two-column CSV of time (s) and stress (MPa).

    Times must be strictly increasing; stresses must be finite.
    A header row named ``time,stress`` (case-insensitive) is optional.
    """
    points: list[Point] = []
    with open(path, newline="", encoding="utf-8-sig") as handle:
        for lineno, row in enumerate(csv.reader(handle), start=1):
            if not row or all(not cell.strip() for cell in row):
                continue
            if lineno == 1 and row[0].strip().lower() in ("time", "t"):
                continue
            if len(row) < 2:
                raise ValueError(f"line {lineno}: expected time,stress columns")
            try:
                time = float(row[0])
                stress = float(row[1])
            except ValueError as exc:
                raise ValueError(f"line {lineno}: non-numeric value") from exc
            if not (math.isfinite(time) and math.isfinite(stress)):
                raise ValueError(f"line {lineno}: values must be finite")
            if points and time <= points[-1].time:
                raise ValueError(f"line {lineno}: time must be strictly increasing")
            points.append(Point(time, stress))
    if not points:
        raise ValueError("CSV contains no data rows")
    return points


def extract_turning_points(points: list[Point]) -> list[Point]:
    """Collapse plateaus and keep endpoints plus direction reversals.

    Consecutive equal stresses keep only the last point of the run. From the
    deduplicated series, keep the first and last points and every point where
    the stress direction reverses. A constant record yields a single point
    (zero cycles, zero damage downstream).
    """
    deduped: list[Point] = []
    for point in points:
        if deduped and point.stress == deduped[-1].stress:
            deduped[-1] = point
        else:
            deduped.append(point)
    if len(deduped) <= 2:
        return deduped
    turning = [deduped[0]]
    for index in range(1, len(deduped) - 1):
        prev_stress = deduped[index - 1].stress
        stress = deduped[index].stress
        next_stress = deduped[index + 1].stress
        if (stress - prev_stress) * (next_stress - stress) < 0:
            turning.append(deduped[index])
    turning.append(deduped[-1])
    return turning
