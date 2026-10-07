"""Projection CSV loading and angular coverage validation."""

from __future__ import annotations

import csv
import math

from .config import MAX_ANGLES

ANGLE_TOL_DEG = 1e-6


class ProjectionError(ValueError):
    """Raised when projection data is malformed or inconsistent."""


def load_projections(path: str, detector_count: int) -> list[tuple[float, list[float]]]:
    """Load projections from CSV.

    Each row is ``angle_deg, d0, d1, ..., d{n-1}`` with exactly
    ``detector_count`` readings.  Angles must uniformly cover
    [0, 180) degrees with no missing or duplicated rows.

    Returns a list of ``(angle_radians, readings)`` sorted by angle.
    """
    rows: list[tuple[float, list[float]]] = []
    with open(path, "r", encoding="utf-8", newline="") as fh:
        for lineno, record in enumerate(csv.reader(fh), start=1):
            if not record or all(not cell.strip() for cell in record):
                continue
            if record[0].strip().startswith("#"):
                continue
            try:
                angle = float(record[0])
            except ValueError as exc:
                raise ProjectionError(
                    f"line {lineno}: invalid angle {record[0]!r}"
                ) from exc
            values = []
            for cell in record[1:]:
                try:
                    values.append(float(cell))
                except ValueError as exc:
                    raise ProjectionError(
                        f"line {lineno}: invalid detector reading {cell!r}"
                    ) from exc
            if len(values) != detector_count:
                raise ProjectionError(
                    f"line {lineno}: expected {detector_count} readings, "
                    f"got {len(values)}"
                )
            rows.append((angle, values))

    if not rows:
        raise ProjectionError("projection file contains no data rows")
    if len(rows) > MAX_ANGLES:
        raise ProjectionError(
            f"{len(rows)} angles exceed limit {MAX_ANGLES}"
        )

    angles = [a for a, _ in rows]
    for a in angles:
        if not (0.0 <= a < 180.0):
            raise ProjectionError(f"angle {a} outside [0, 180) degrees")
    for i, a in enumerate(angles):
        for b in angles[i + 1 :]:
            if abs(a - b) <= ANGLE_TOL_DEG:
                raise ProjectionError(f"duplicate angle {a} degrees")

    n = len(angles)
    step = 180.0 / n
    expected = {round(i * step, 9) for i in range(n)}
    actual = {round(a, 9) for a in angles}
    if expected != actual:
        missing = sorted(expected - actual)
        raise ProjectionError(
            f"angles must uniformly cover [0, 180) with step {step} deg; "
            f"missing/non-uniform angles, e.g. {missing[:3]}"
        )

    rows.sort(key=lambda r: r[0])
    return [(math.radians(a), v) for a, v in rows]
