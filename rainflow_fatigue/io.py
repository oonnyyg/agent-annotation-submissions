"""Input/output helpers: CSV history, JSON config, reports."""
from __future__ import annotations

import csv
import json
import math
from pathlib import Path

from .damage import CycleDamage, DamageModel
from .turning_points import Point


def load_history(path: str | Path) -> list[Point]:
    """Load a CSV with strictly increasing time and finite signed stress (MPa)."""
    points: list[Point] = []
    with open(path, newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames is None or not {"time", "stress"} <= set(reader.fieldnames):
            raise ValueError("CSV must have 'time' and 'stress' columns")
        for row_number, row in enumerate(reader, start=2):
            time = float(row["time"])
            stress = float(row["stress"])
            if not (math.isfinite(time) and math.isfinite(stress)):
                raise ValueError(f"row {row_number}: non-finite value")
            if points and time <= points[-1].time:
                raise ValueError(f"row {row_number}: time not strictly increasing")
            points.append(Point(time, stress))
    if not points:
        raise ValueError("CSV contains no samples")
    return points


def load_model(path: str | Path) -> DamageModel:
    """Load the damage model from a JSON config file."""
    with open(path, encoding="utf-8") as handle:
        config = json.load(handle)
    return DamageModel(
        reference_stress=float(config["reference_stress"]),
        exponent=float(config["exponent"]),
        mean_correction=config.get("mean_correction", "none"),
        ultimate_strength=(
            None
            if config.get("ultimate_strength") is None
            else float(config["ultimate_strength"])
        ),
    )


def _cell(value: float | None) -> object:
    return "" if value is None else value


def write_cycles_csv(results: list[CycleDamage], path: str | Path) -> None:
    """Write one row per counted cycle, keeping endpoint times."""
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow([
            "start_time", "end_time", "stress_range", "amplitude", "mean",
            "count", "evaluable", "equivalent_amplitude", "life", "damage",
        ])
        for result in results:
            cycle = result.cycle
            writer.writerow([
                cycle.start_time, cycle.end_time, cycle.stress_range,
                cycle.amplitude, cycle.mean, cycle.count,
                result.evaluable,
                _cell(result.equivalent_amplitude),
                _cell(result.life),
                _cell(result.damage),
            ])
