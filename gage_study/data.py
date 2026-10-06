"""Loading and validation of measurement data."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass


class GageStudyError(Exception):
    """Raised for any invalid input data or configuration."""


@dataclass(frozen=True)
class Config:
    part_column: str = "part"
    operator_column: str = "operator"
    replicate_column: str = "replicate"
    value_column: str = "value"


def load_config(path: str) -> Config:
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    if not isinstance(raw, dict):
        raise GageStudyError("config JSON must be an object")
    allowed = {"part_column", "operator_column", "replicate_column", "value_column"}
    unknown = set(raw) - allowed
    if unknown:
        raise GageStudyError(f"unknown config keys: {sorted(unknown)}")
    kwargs = {}
    for key, value in raw.items():
        if not isinstance(value, str) or not value:
            raise GageStudyError(f"config value for {key!r} must be a non-empty string")
        kwargs[key] = value
    return Config(**kwargs)


@dataclass(frozen=True)
class MeasurementData:
    # cells[(part, operator)] = list of values ordered by replicate id
    cells: dict
    parts: tuple
    operators: tuple
    replicates: int


def load_measurements(path: str, config: Config) -> MeasurementData:
    rows = []
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        required = {
            config.part_column,
            config.operator_column,
            config.replicate_column,
            config.value_column,
        }
        missing = required - set(reader.fieldnames or [])
        if missing:
            raise GageStudyError(f"missing CSV columns: {sorted(missing)}")
        for lineno, row in enumerate(reader, start=2):
            part = (row[config.part_column] or "").strip()
            operator = (row[config.operator_column] or "").strip()
            rep_text = (row[config.replicate_column] or "").strip()
            val_text = (row[config.value_column] or "").strip()
            if not part or not operator:
                raise GageStudyError(f"line {lineno}: empty part or operator")
            try:
                value = float(val_text)
            except ValueError:
                raise GageStudyError(
                    f"line {lineno}: non-numeric value {val_text!r}"
                ) from None
            if not math.isfinite(value):
                raise GageStudyError(f"line {lineno}: non-finite value {val_text!r}")
            rows.append((part, operator, rep_text, value))

    if not rows:
        raise GageStudyError("no measurement rows found")

    parts = sorted({r[0] for r in rows})
    operators = sorted({r[1] for r in rows})
    if len(parts) < 2:
        raise GageStudyError("at least two distinct parts are required")
    if len(operators) < 2:
        raise GageStudyError("at least two distinct operators are required")

    seen = set()
    cells: dict = {}
    for part, operator, rep, value in rows:
        key = (part, operator, rep)
        if key in seen:
            raise GageStudyError(
                f"duplicate replicate id {rep!r} for part={part!r} operator={operator!r}"
            )
        seen.add(key)
        cells.setdefault((part, operator), {})[rep] = value

    rep_counts = {len(v) for v in cells.values()}
    if len(rep_counts) != 1:
        raise GageStudyError("unbalanced data: replicate counts differ across cells")
    n_reps = rep_counts.pop()
    if n_reps < 2:
        raise GageStudyError("each part/operator cell needs at least two replicates")

    for part in parts:
        for operator in operators:
            if (part, operator) not in cells:
                raise GageStudyError(
                    f"missing cell: part={part!r} operator={operator!r}"
                )

    sorted_cells = {
        key: [cells[key][rep] for rep in sorted(cells[key])]
        for key in sorted(cells)
    }
    return MeasurementData(
        cells=sorted_cells,
        parts=tuple(parts),
        operators=tuple(operators),
        replicates=n_reps,
    )
