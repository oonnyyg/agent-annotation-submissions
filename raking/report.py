"""Output writers: success CSVs or a failure diagnostics JSON."""

from __future__ import annotations

import csv
import json
import os

from .core import STATUS_CONVERGED, RakingResult, effective_sample_size
from .errors import RakingError
from .inputs import Margin, Sample

WEIGHTS_CSV = "weights.csv"
MARGINS_CSV = "margins.csv"
DIAGNOSTICS_JSON = "diagnostics.json"


def write_success(out_dir: str, samples: list[Sample], margins: list[Margin],
                  result: RakingResult) -> dict[str, str]:
    """Write per-sample weights and per-margin target/achieved tables."""
    if result.status != STATUS_CONVERGED:
        raise RakingError("write_success called with a non-converged result")
    os.makedirs(out_dir, exist_ok=True)
    weights_path = os.path.join(out_dir, WEIGHTS_CSV)
    margins_path = os.path.join(out_dir, MARGINS_CSV)

    with open(weights_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["id", "initial_weight", "final_weight", "multiplier"])
        for sample, weight in zip(samples, result.weights):
            writer.writerow([
                sample.sample_id,
                repr(sample.initial_weight),
                repr(weight),
                repr(weight / sample.initial_weight),
            ])

    ess = effective_sample_size(result.weights)
    with open(margins_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["margin", "category", "target", "achieved",
                         "abs_residual"])
        for margin in margins:
            for key, target in margin.targets.items():
                achieved = result.achieved.get(margin.name, {}).get(key, 0.0)
                writer.writerow([margin.name, key, repr(target),
                                 repr(achieved), repr(abs(achieved - target))])
        writer.writerow([])
        writer.writerow(["status", result.status])
        writer.writerow(["iterations", result.iterations])
        writer.writerow(["effective_sample_size", repr(ess)])
        writer.writerow(["weight_total", repr(sum(result.weights))])
    return {"weights": weights_path, "margins": margins_path}


def write_diagnostics(out_dir: str, result: RakingResult,
                      note: str | None = None) -> str:
    """Write a failure diagnostics file. Never touches success outputs."""
    if result.status == STATUS_CONVERGED:
        raise RakingError("write_diagnostics called with a converged result")
    os.makedirs(out_dir, exist_ok=True)
    path = os.path.join(out_dir, DIAGNOSTICS_JSON)
    payload = {
        "status": result.status,
        "converged": False,
        "iterations": result.iterations,
        "note": note,
        "failures": [
            {"iteration": f.iteration, "margin": f.margin,
             "category": f.category, "reason": f.reason}
            for f in result.failures
        ],
        "history": [
            {"iteration": h.iteration, "max_residual": h.max_residual,
             "margin": h.margin, "category": h.category}
            for h in result.history
        ],
    }
    if result.achieved:
        payload["achieved_at_stop"] = result.achieved
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    return path
