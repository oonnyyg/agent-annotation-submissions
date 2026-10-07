"""CSV grid loading and validation.

Mask convention: a mask cell value of ``0`` means the pixel is valid and
participates in unwrapping; any non-zero value means the pixel is masked
out and excluded from all computation.
"""

from __future__ import annotations

import csv
import math
from dataclasses import dataclass

TWO_PI = 2.0 * math.pi


def normalize_phase(value: float) -> float:
    """Normalize a phase value into the half-open interval [-pi, pi)."""
    wrapped = math.fmod(value + math.pi, TWO_PI)
    if wrapped < 0.0:
        wrapped += TWO_PI
    result = wrapped - math.pi
    if result >= math.pi:
        result = -math.pi
    return result


def _read_csv_grid(path: str) -> list[list[float]]:
    rows: list[list[float]] = []
    with open(path, newline="", encoding="utf-8") as handle:
        for line_no, record in enumerate(csv.reader(handle), start=1):
            if not record or all(cell.strip() == "" for cell in record):
                continue
            try:
                row = [float(cell) for cell in record]
            except ValueError as exc:
                raise ValueError(
                    f"{path}: line {line_no}: non-numeric cell: {exc}"
                ) from exc
            if rows and len(row) != len(rows[0]):
                raise ValueError(
                    f"{path}: line {line_no}: ragged row "
                    f"({len(row)} cells, expected {len(rows[0])})"
                )
            rows.append(row)
    if not rows:
        raise ValueError(f"{path}: empty grid")
    return rows


@dataclass
class GridData:
    """Validated input grids.

    ``phase`` holds normalized phases in [-pi, pi) for valid pixels and
    ``nan`` for masked pixels; ``quality`` likewise uses ``nan`` for
    masked pixels.
    """

    rows: int
    cols: int
    phase: list[list[float]]
    quality: list[list[float]]
    valid: list[list[bool]]


def load_grids(phase_path: str, quality_path: str, mask_path: str) -> GridData:
    phase = _read_csv_grid(phase_path)
    quality = _read_csv_grid(quality_path)
    mask = _read_csv_grid(mask_path)

    rows = len(phase)
    cols = len(phase[0])
    for name, grid in (("quality", quality), ("mask", mask)):
        if len(grid) != rows or len(grid[0]) != cols:
            raise ValueError(
                f"{name} grid shape {len(grid)}x{len(grid[0])} does not "
                f"match phase grid shape {rows}x{cols}"
            )

    out_phase: list[list[float]] = []
    out_quality: list[list[float]] = []
    valid: list[list[bool]] = []
    for r in range(rows):
        prow: list[float] = []
        qrow: list[float] = []
        vrow: list[bool] = []
        for c in range(cols):
            is_valid = mask[r][c] == 0.0
            vrow.append(is_valid)
            if not is_valid:
                prow.append(math.nan)
                qrow.append(math.nan)
                continue
            p = phase[r][c]
            q = quality[r][c]
            if not math.isfinite(p):
                raise ValueError(
                    f"phase[{r}][{c}] is not finite on a valid pixel"
                )
            if not math.isfinite(q):
                raise ValueError(
                    f"quality[{r}][{c}] is not finite on a valid pixel"
                )
            if q < 0.0:
                raise ValueError(f"quality[{r}][{c}] is negative")
            prow.append(normalize_phase(p))
            qrow.append(q)
        out_phase.append(prow)
        out_quality.append(qrow)
        valid.append(vrow)

    return GridData(rows=rows, cols=cols, phase=out_phase,
                    quality=out_quality, valid=valid)
