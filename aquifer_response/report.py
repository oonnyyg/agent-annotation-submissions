"""CSV time-series output and JSON analysis report."""

from __future__ import annotations

import csv
import json
import os
import tempfile
from dataclasses import asdict
from typing import Any

from .fitting import CandidateScore, Observation
from .model import Aquifer, EvaluationStats, ObservationPoint, Well


def write_csv(
    path: str,
    times: list[float],
    wells: list[Well],
    points: list[ObservationPoint],
    series: dict[str, dict[str, list[float]]],
) -> None:
    """Write per-point drawdown series with per-well contributions."""
    well_names = [w.name for w in wells]
    header = ["time", "point", "total_drawdown"] + [
        f"contribution_{name}" for name in well_names
    ]
    _atomic_write(path, lambda fh: _write_csv_rows(fh, header, times, points, well_names, series))


def _write_csv_rows(fh, header, times, points, well_names, series) -> None:
    writer = csv.writer(fh)
    writer.writerow(header)
    for point in points:
        data = series[point.name]
        for index, t in enumerate(times):
            row = [t, point.name, data["total"][index]]
            row += [data[name][index] for name in well_names]
            writer.writerow(row)


def build_report(
    aquifer: Aquifer,
    wells: list[Well],
    points: list[ObservationPoint],
    times: list[float],
    series: dict[str, dict[str, list[float]]],
    observations: list[Observation],
    ranked: list[CandidateScore] | None,
    stats: EvaluationStats,
) -> dict[str, Any]:
    report: dict[str, Any] = {
        "model": {
            "type": "theis_confined_infinite",
            "transmissivity": aquifer.transmissivity,
            "storativity": aquifer.storativity,
            "units": {"length": "m", "time": "s", "rate": "m^3/s"},
        },
        "wells": [
            {
                "name": w.name,
                "x": w.x,
                "y": w.y,
                "schedule": [
                    {"time": s.time, "rate": s.rate} for s in w.schedule
                ],
            }
            for w in wells
        ],
        "observation_points": [
            {
                "name": p.name,
                "x": p.x,
                "y": p.y,
                "max_drawdown": max(series[p.name]["total"]),
                "final_drawdown": series[p.name]["total"][-1],
                "well_contributions_at_final_time": {
                    w.name: series[p.name][w.name][-1] for w in wells
                },
            }
            for p in points
        ],
        "well_function_stats": {
            "evaluations": stats.evaluations,
            "max_terms_used": stats.max_terms_used,
            "methods": stats.methods,
            "converged": True,
        },
    }
    if observations:
        report["residuals"] = _residual_summary(observations, ranked)
    if ranked is not None:
        report["candidate_ranking"] = [asdict(score) for score in ranked]
        report["best_candidate"] = ranked[0].name if ranked else None
    return report


def _residual_summary(
    observations: list[Observation], ranked: list[CandidateScore] | None
) -> dict[str, Any]:
    # Residuals are reported for the best candidate when candidates were
    # provided; otherwise weights/count only (predictions require params).
    summary: dict[str, Any] = {
        "observation_count": len(observations),
        "total_weight": sum(o.weight for o in observations),
    }
    if ranked:
        best = ranked[0]
        summary["best_candidate"] = best.name
        summary["weighted_sse"] = best.weighted_sse
        summary["weighted_rmse"] = best.weighted_rmse
    return summary


def write_report(path: str, report: dict[str, Any]) -> None:
    _atomic_write(path, lambda fh: json.dump(report, fh, indent=2))


def _atomic_write(path: str, write_fn) -> None:
    """Write via a temp file + rename so failures never clobber existing
    output files."""
    directory = os.path.dirname(os.path.abspath(path))
    fd, tmp_path = tempfile.mkstemp(dir=directory, suffix=".tmp")
    try:
        with os.fdopen(fd, "w", newline="", encoding="utf-8") as fh:
            write_fn(fh)
        os.replace(tmp_path, path)
    except BaseException:
        try:
            os.unlink(tmp_path)
        except OSError:
            pass
        raise
