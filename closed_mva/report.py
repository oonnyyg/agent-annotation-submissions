"""CSV and JSON rendering of an analysis report."""

from __future__ import annotations

import csv
import json
from pathlib import Path

from .analysis import AnalysisReport


def _ensure_parent(path: str | Path) -> Path:
    p = Path(path)
    if str(p.parent) not in ("", "."):
        p.parent.mkdir(parents=True, exist_ok=True)
    return p


def write_csv(report: AnalysisReport, path: str | Path) -> None:
    """One row per (scenario, population level, station)."""
    with open(_ensure_parent(path), "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow([
            "scenario", "population", "station", "station_type",
            "mean_service_time", "visit_ratio", "residence_time",
            "queue_length", "load", "load_kind",
            "reference_throughput", "cycle_time",
        ])
        for sc in report.scenarios:
            for point in sc.curve:
                for st in point.stations:
                    writer.writerow([
                        sc.name, point.population, st.name, st.kind,
                        f"{st.mean_service_time:.10g}", f"{st.visit_ratio:.10g}",
                        f"{st.residence_time:.10g}", f"{st.queue_length:.10g}",
                        f"{st.load:.10g}", st.load_kind,
                        f"{point.throughput:.10g}", f"{point.cycle_time:.10g}",
                    ])


def build_summary(report: AnalysisReport) -> dict:
    scenarios = []
    for sc in report.scenarios:
        stations = {}
        for st in sc.final.stations:
            stations[st.name] = {
                "type": st.kind,
                "mean_service_time": st.mean_service_time,
                "visit_ratio": st.visit_ratio,
                "residence_time": st.residence_time,
                "queue_length": st.queue_length,
                "load": st.load,
                "load_kind": st.load_kind,
            }
        scenarios.append({
            "name": sc.name,
            "population": sc.population,
            "reference_throughput": sc.final.throughput,
            "cycle_time": sc.final.cycle_time,
            "throughput_change_vs_baseline": sc.throughput_change_vs_baseline,
            "queue_length_total": sum(s.queue_length for s in sc.final.stations),
            "stations": stations,
        })
    return {
        "reference_station": report.reference_station,
        "visit_ratios": report.visit_ratios,
        "baseline_scenario": report.scenarios[0].name if report.scenarios else None,
        "scenarios": scenarios,
    }


def write_json(report: AnalysisReport, path: str | Path) -> None:
    _ensure_parent(path).write_text(
        json.dumps(build_summary(report), indent=2) + "\n", encoding="utf-8")
