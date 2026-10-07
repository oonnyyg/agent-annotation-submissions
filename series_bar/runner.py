"""Load inputs, run the load history, and emit CSV/JSON outputs."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass
from pathlib import Path

from .material import MaterialParams
from .solver import BarModel, Element, StepResult


@dataclass
class RunConfig:
    tol_force: float = 1.0e-8
    tol_disp: float = 1.0e-12
    max_iter: int = 50


def load_model(config_path: str | Path) -> BarModel:
    with open(config_path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, dict) or "elements" not in data:
        raise ValueError("config JSON must be an object with an 'elements' list")
    elements_data = data["elements"]
    if not isinstance(elements_data, list) or not elements_data:
        raise ValueError("'elements' must be a non-empty list")
    elements = []
    for i, item in enumerate(elements_data):
        if not isinstance(item, dict):
            raise ValueError(f"element {i}: must be an object")
        try:
            material = MaterialParams(
                youngs_modulus=float(item["youngs_modulus"]),
                yield_strength=float(item["yield_strength"]),
                hardening_modulus=float(item["hardening_modulus"]),
            )
            element = Element(
                length=float(item["length"]),
                area=float(item["area"]),
                material=material,
            )
        except KeyError as exc:
            raise ValueError(f"element {i}: missing key {exc}") from exc
        except (TypeError, ValueError) as exc:
            raise ValueError(f"element {i}: invalid numeric value: {exc}") from exc
        elements.append(element)
    return BarModel(elements)


def load_displacements(csv_path: str | Path) -> list[float]:
    values: list[float] = []
    with open(csv_path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.reader(fh)
        for row_no, row in enumerate(reader, start=1):
            if not row or all(not cell.strip() for cell in row):
                continue
            first = row[0].strip()
            if row_no == 1 and first.lower() in ("displacement", "u", "u_end"):
                continue
            try:
                value = float(first)
            except ValueError:
                raise ValueError(
                    f"displacement CSV row {row_no}: not a number: {first!r}"
                ) from None
            if not math.isfinite(value):
                raise ValueError(
                    f"displacement CSV row {row_no}: non-finite value {first!r}"
                )
            values.append(value)
    if not values:
        raise ValueError("displacement CSV contains no load steps")
    return values


def run_history(
    model: BarModel, displacements: list[float], config: RunConfig
) -> tuple[list[StepResult], StepResult | None]:
    """Run steps; stop at first failure. Returns (results, failed_or_None)."""
    results: list[StepResult] = []
    for step, u_end in enumerate(displacements, start=1):
        result = model.solve_step(
            step, u_end, config.tol_force, config.tol_disp, config.max_iter
        )
        if not result.converged:
            return results, result
        results.append(result)
    return results, None
