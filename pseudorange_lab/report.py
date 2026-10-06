"""Per-epoch processing and JSON/CSV report generation.

Epochs are processed independently: one failure never blocks the others.
"""
from __future__ import annotations

import csv
import json
from typing import Any

from .data import Epoch
from .errors import ConvergenceError, GeometryError
from .sensitivity import leave_one_out
from .solver import solve_epoch


def process_epochs(epochs: dict[str, Epoch]) -> dict[str, Any]:
    """Solve every epoch and build a JSON-serializable report."""
    report: dict[str, Any] = {"epochs": []}
    for name in sorted(epochs):
        epoch = epochs[name]
        entry: dict[str, Any] = {"epoch": name}
        try:
            sol = solve_epoch(
                epoch.observations,
                epoch.initial_position_m,
                epoch.initial_clock_m,
            )
        except (GeometryError, ConvergenceError) as exc:
            entry.update(status="error", detail=str(exc))
            report["epochs"].append(entry)
            continue
        entry.update(
            status="ok",
            position_m=list(sol.position_m),
            clock_m=sol.clock_m,
            iterations=sol.iterations,
            weighted_residual_sum_squares=sol.weighted_residual_sum_squares,
            covariance=sol.covariance,
            residuals=[
                {"sat_id": sid, "residual_m": res}
                for sid, res in zip(sol.sat_ids, sol.residuals_m)
            ],
        )
        loo = leave_one_out(
            epoch.observations,
            epoch.initial_position_m,
            epoch.initial_clock_m,
            sol,
        )
        if loo is not None:
            entry["leave_one_out"] = [
                {
                    "excluded_sat_id": r.excluded_sat_id,
                    "status": r.status,
                    "detail": r.detail,
                    "position_shift_m": r.position_shift_m,
                    "clock_shift_m": r.clock_shift_m,
                    "extrapolated_residual_m": r.extrapolated_residual_m,
                }
                for r in loo
            ]
        report["epochs"].append(entry)
    return report


def write_json_report(report: dict[str, Any], path: str) -> None:
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(report, handle, indent=2, ensure_ascii=False)
        handle.write("\n")


def write_csv_report(report: dict[str, Any], path: str) -> None:
    """Flat CSV: one row per epoch solution, plus per-satellite residual rows."""
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow([
            "epoch", "row_type", "sat_id", "x_m", "y_m", "z_m", "clock_m",
            "residual_m", "wrss", "iterations", "status", "detail",
        ])
        for entry in report["epochs"]:
            if entry["status"] != "ok":
                writer.writerow([entry["epoch"], "solution", "", "", "", "",
                                 "", "", "", "", "error", entry["detail"]])
                continue
            pos = entry["position_m"]
            writer.writerow([
                entry["epoch"], "solution", "", pos[0], pos[1], pos[2],
                entry["clock_m"], "",
                entry["weighted_residual_sum_squares"],
                entry["iterations"], "ok", "",
            ])
            for res in entry["residuals"]:
                writer.writerow([entry["epoch"], "residual", res["sat_id"],
                                 "", "", "", "", res["residual_m"],
                                 "", "", "", ""])
            for loo in entry.get("leave_one_out", []):
                writer.writerow([
                    entry["epoch"], "leave_one_out", loo["excluded_sat_id"],
                    "", "", "", loo["clock_shift_m"],
                    loo["extrapolated_residual_m"],
                    loo["position_shift_m"], "", loo["status"],
                    loo["detail"] or "",
                ])
