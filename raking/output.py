"""Output writers: success weights/margins files and failure diagnostics."""

from __future__ import annotations

import csv
import json
from pathlib import Path

from .core import Margin, RakingResult


def _key(category: tuple[str, ...]) -> str:
    return "||".join(category)


def write_success(
    out_dir: str | Path,
    ids: list[str],
    initial_weights: list[float],
    margins: list[Margin],
    result: RakingResult,
) -> tuple[Path, Path]:
    """Write per-sample weights CSV and margins JSON. Returns their paths."""
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    weights_path = out / "weights.csv"
    with open(weights_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["sample_id", "initial_weight", "final_weight", "multiplier"])
        for sample_id, initial, final in zip(ids, initial_weights, result.weights):
            writer.writerow([sample_id, repr(initial), repr(final), repr(final / initial)])
    margins_path = out / "margins.json"
    payload = {
        "status": "converged",
        "rounds": len(result.rounds),
        "effective_sample_size": result.effective_sample_size,
        "margins": [
            {
                "name": margin.name,
                "fields": list(margin.fields),
                "categories": [
                    {
                        "category": _key(category),
                        "target": target,
                        "achieved": result.achieved[margin.name][category],
                    }
                    for category, target in margin.targets.items()
                ],
            }
            for margin in margins
        ],
    }
    with open(margins_path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, ensure_ascii=False, indent=2)
    return weights_path, margins_path


def write_failure(
    out_dir: str | Path,
    margins: list[Margin],
    result: RakingResult,
) -> Path:
    """Write a status=failed diagnostic file without touching success outputs."""
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    diag_path = out / "diagnostics.json"
    payload = {
        "status": "failed",
        "failure_reason": result.failure_reason,
        "rounds_completed": len(result.rounds),
        "round_diagnostics": [
            {
                "round": diag.round_index,
                "max_residual": diag.max_residual,
                "worst_margin": diag.worst_margin,
                "worst_category": _key(diag.worst_category),
            }
            for diag in result.rounds
        ],
        "margins": [
            {
                "name": margin.name,
                "fields": list(margin.fields),
                "categories": [
                    {
                        "category": _key(category),
                        "target": target,
                        "achieved": result.achieved[margin.name][category],
                    }
                    for category, target in margin.targets.items()
                ],
            }
            for margin in margins
        ],
    }
    with open(diag_path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, ensure_ascii=False, indent=2)
    return diag_path
