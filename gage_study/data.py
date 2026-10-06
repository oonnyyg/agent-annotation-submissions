"""Reading and strict validation of measurement CSV and JSON config."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass


class InputError(ValueError):
    """Raised when the input data or config is invalid."""


DEFAULT_COLUMNS = {
    "part": "part",
    "operator": "operator",
    "replicate": "replicate",
    "value": "value",
}


@dataclass(frozen=True)
class Observation:
    part: str
    operator: str
    replicate: int
    value: float


def load_config(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as fh:
        config = json.load(fh)
    if not isinstance(config, dict):
        raise InputError("config JSON must be an object")
    columns = config.get("columns", {})
    if not isinstance(columns, dict):
        raise InputError("config 'columns' must be an object")
    merged = dict(DEFAULT_COLUMNS)
    for key, col in columns.items():
        if key not in DEFAULT_COLUMNS:
            raise InputError(f"unknown column key in config: {key!r}")
        if not isinstance(col, str) or not col:
            raise InputError(f"column name for {key!r} must be a non-empty string")
        merged[key] = col
    return {"columns": merged}


def load_observations(csv_path: str, columns: dict) -> list[Observation]:
    observations: list[Observation] = []
    with open(csv_path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        if reader.fieldnames is None:
            raise InputError("measurement CSV has no header row")
        missing = [c for c in columns.values() if c not in reader.fieldnames]
        if missing:
            raise InputError(f"missing CSV columns: {', '.join(sorted(set(missing)))}")
        for lineno, row in enumerate(reader, start=2):
            part = (row.get(columns["part"]) or "").strip()
            operator = (row.get(columns["operator"]) or "").strip()
            rep_raw = (row.get(columns["replicate"]) or "").strip()
            val_raw = (row.get(columns["value"]) or "").strip()
            if not part or not operator:
                raise InputError(f"line {lineno}: empty part or operator")
            try:
                replicate = int(rep_raw)
            except ValueError:
                raise InputError(f"line {lineno}: replicate {rep_raw!r} is not an integer")
            try:
                value = float(val_raw)
            except ValueError:
                raise InputError(f"line {lineno}: value {val_raw!r} is not a number")
            if not math.isfinite(value):
                raise InputError(f"line {lineno}: value {val_raw!r} is not finite")
            observations.append(Observation(part, operator, replicate, value))
    if not observations:
        raise InputError("measurement CSV contains no observations")
    return observations


def validate_structure(observations: list[Observation]) -> tuple[list[str], list[str], int]:
    """Check balance/duplicates; return sorted parts, operators, replicate count."""
    parts = sorted({o.part for o in observations})
    operators = sorted({o.operator for o in observations})
    if len(parts) < 2:
        raise InputError("at least two distinct parts are required")
    if len(operators) < 2:
        raise InputError("at least two distinct operators are required")

    cells: dict[tuple[str, str], set[int]] = {}
    for obs in observations:
        key = (obs.part, obs.operator)
        reps = cells.setdefault(key, set())
        if obs.replicate in reps:
            raise InputError(
                f"duplicate replicate {obs.replicate} for part={obs.part!r} "
                f"operator={obs.operator!r}"
            )
        reps.add(obs.replicate)

    counts = {key: len(reps) for key, reps in cells.items()}
    for part in parts:
        for operator in operators:
            if (part, operator) not in counts:
                raise InputError(f"missing cell: part={part!r} operator={operator!r}")
    distinct_counts = set(counts.values())
    if len(distinct_counts) != 1:
        raise InputError("unbalanced design: replicate counts differ across cells")
    n_replicates = distinct_counts.pop()
    if n_replicates < 2:
        raise InputError("each part/operator cell needs at least two replicates")
    return parts, operators, n_replicates
