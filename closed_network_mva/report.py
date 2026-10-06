"""CSV and JSON reporting for MVA scenario results."""

from __future__ import annotations

import csv
import json
from pathlib import Path

from .mva import ScenarioResult
from .model import NetworkConfig

_CSV_FIELDS = [
    "scenario",
    "population",
    "station",
    "type",
    "mean_service_time",
    "visit_ratio",
    "residence_time",
    "mean_queue_length",
    "load",
    "load_meaning",
    "station_throughput",
]


def write_csv(results: list[ScenarioResult], path: str | Path) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=_CSV_FIELDS)
        writer.writeheader()
        for result in results:
            for station in result.stations:
                writer.writerow({
                    "scenario": result.name,
                    "population": result.population,
                    "station": station.name,
                    "type": station.kind,
                    "mean_service_time": f"{station.mean_service_time:.10g}",
                    "visit_ratio": f"{station.visit_ratio:.10g}",
                    "residence_time": f"{station.residence_time:.10g}",
                    "mean_queue_length": f"{station.mean_queue_length:.10g}",
                    "load": f"{station.load:.10g}",
                    "load_meaning": ("utilization" if station.kind == "single"
                                     else "mean_busy_servers"),
                    "station_throughput": f"{result.throughput * station.visit_ratio:.10g}",
                })


def build_summary(config: NetworkConfig, results: list[ScenarioResult]) -> dict:
    scenarios = []
    for result in results:
        scenarios.append({
            "name": result.name,
            "population": result.population,
            "reference_station": config.reference_station,
            "throughput": result.throughput,
            "cycle_time": result.cycle_time,
            "relative_throughput": result.relative_throughput,
            "queue_length_sum": sum(s.mean_queue_length for s in result.stations),
            "zero_population_queues": {
                s.name: q for s, q in zip(config.stations, result.zero_population_queues)
            },
            "stations": [
                {
                    "name": s.name,
                    "type": s.kind,
                    "mean_service_time": s.mean_service_time,
                    "visit_ratio": s.visit_ratio,
                    "residence_time": s.residence_time,
                    "mean_queue_length": s.mean_queue_length,
                    "load": s.load,
                    "load_meaning": ("utilization" if s.kind == "single"
                                     else "mean_busy_servers"),
                }
                for s in result.stations
            ],
        })
    return {
        "reference_station": config.reference_station,
        "baseline_scenario": results[0].name if results else None,
        "scenarios": scenarios,
    }


def write_json(config: NetworkConfig, results: list[ScenarioResult], path: str | Path) -> None:
    summary = build_summary(config, results)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(summary, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
