"""CSV result writer and JSON diagnostics builder."""

from __future__ import annotations

import csv
import json
from pathlib import Path

from .solver import StepResult


def write_results_csv(path: str | Path, results: list[StepResult]) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "step",
                "prescribed_u_end",
                "iterations",
                "reaction",
                "node",
                "node_displacement",
                "element",
                "strain",
                "stress",
                "axial_force",
                "eps_p",
                "alpha",
                "force_imbalance",
                "compatibility_residual",
            ]
        )
        for res in results:
            n_nodes = len(res.node_displacements)
            n_rows = max(n_nodes, len(res.elements))
            for i in range(n_rows):
                writer.writerow(
                    [
                        res.step if i == 0 else "",
                        res.prescribed_displacement if i == 0 else "",
                        res.iterations if i == 0 else "",
                        res.reaction if i == 0 else "",
                        i if i < n_nodes else "",
                        res.node_displacements[i] if i < n_nodes else "",
                        i if i < len(res.elements) else "",
                        res.elements[i].strain if i < len(res.elements) else "",
                        res.elements[i].stress if i < len(res.elements) else "",
                        res.elements[i].axial_force if i < len(res.elements) else "",
                        res.elements[i].eps_p if i < len(res.elements) else "",
                        res.elements[i].alpha if i < len(res.elements) else "",
                        res.force_imbalance if i == 0 else "",
                        res.compatibility_residual if i == 0 else "",
                    ]
                )


def build_diagnostics(
    results: list[StepResult],
    failed: StepResult | None,
    tol_force: float,
    tol_disp: float,
    max_iter: int,
) -> dict:
    return {
        "status": "failed" if failed is not None else "converged",
        "converged_steps": len(results),
        "failed_step": failed.step if failed is not None else None,
        "failed_step_iterations": failed.iterations if failed is not None else None,
        "failed_step_force_imbalance": (
            failed.force_imbalance if failed is not None else None
        ),
        "tolerances": {"force": tol_force, "displacement": tol_disp},
        "max_iter": max_iter,
        "steps": [
            {
                "step": r.step,
                "prescribed_u_end": r.prescribed_displacement,
                "iterations": r.iterations,
                "reaction": r.reaction,
                "force_imbalance": r.force_imbalance,
                "compatibility_residual": r.compatibility_residual,
                "max_abs_eps_p": max((abs(e.eps_p) for e in r.elements), default=0.0),
                "max_alpha": max((e.alpha for e in r.elements), default=0.0),
            }
            for r in results
        ],
    }


def write_diagnostics_json(path: str | Path, diagnostics: dict) -> None:
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(diagnostics, fh, indent=2)
        fh.write("\n")
