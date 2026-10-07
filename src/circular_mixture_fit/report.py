"""Output writers: assignment CSV and JSON convergence report."""

from __future__ import annotations

import csv
import json
import math
from pathlib import Path

from .em import Component, FitResult


def sort_components(
    components: list[Component],
) -> tuple[list[Component], list[int]]:
    """Sort components by normalized direction mu; returns (sorted, order).

    ``order[new_index] == old_index`` so responsibility columns can be
    permuted consistently.
    """
    order = sorted(range(len(components)), key=lambda j: components[j].mu)
    return [components[j] for j in order], order


def write_assignments(
    path: str | Path,
    angles: list[float],
    weights: list[float],
    resp: list[list[float]],
    order: list[int],
    unit: str,
) -> None:
    """Per-observation CSV with reordered responsibility columns."""
    scale = 180.0 / math.pi if unit == "deg" else 1.0
    k = len(order)
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        header = ["index", "angle", "weight"] + [
            f"component_{j + 1}_probability" for j in range(k)
        ]
        writer.writerow(header)
        for i, (theta, w) in enumerate(zip(angles, weights)):
            row = [i, f"{theta * scale:.10g}", f"{w:.10g}"]
            row += [f"{resp[i][order[j]]:.10g}" for j in range(k)]
            writer.writerow(row)


def build_report(
    result: FitResult,
    sorted_components: list[Component],
    final_log_likelihood: float,
    unit: str,
) -> dict:
    scale = 180.0 / math.pi if unit == "deg" else 1.0
    return {
        "converged": result.converged,
        "status": result.message,
        "iterations": result.iterations,
        "log_likelihood": final_log_likelihood,
        "log_likelihood_history": result.log_likelihood_history,
        "unit": unit,
        "components": [
            {
                "weight": comp.weight,
                "mu": comp.mu * scale,
                "kappa": comp.kappa,
            }
            for comp in sorted_components
        ],
    }


def write_report(path: str | Path, report: dict) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)
        fh.write("\n")
