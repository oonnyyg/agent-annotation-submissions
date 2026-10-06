"""Command line interface: coils JSON + probes CSV -> JSON/CSV reports."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .model import CoilDataError, load_coils
from .probe import ProbeDataError, load_probes
from .report import build_report
from .scenario import ScenarioDataError, baseline_scenario, load_scenarios


def _flatten_rows(report: dict) -> list[dict[str, str]]:
    rows = []
    for scheme in report["scenarios"]:
        for point in scheme["points"]:
            row: dict[str, str] = {
                "scenario": scheme["name"],
                "probe": point["probe"],
                "computable": str(point["computable"]),
            }
            if not point["computable"]:
                row["blocking_coils"] = ";".join(point["blocking_coils"])
                rows.append(row)
                continue
            for label, vec in (
                ("world", point["total_world_T"]),
                ("local", point["total_local_T"]),
            ):
                for axis, value in zip("xyz", vec):
                    row[f"{label}_{axis}_T"] = f"{value:.10e}"
            row["magnitude_T"] = f"{point['magnitude_T']:.10e}"
            delta = point.get("delta_vs_baseline")
            if delta:
                for axis, value in zip("xyz", delta["world_T"]):
                    row[f"delta_world_{axis}_T"] = f"{value:.10e}"
                for axis, value in zip("xyz", delta["local_T"]):
                    row[f"delta_local_{axis}_T"] = f"{value:.10e}"
                row["delta_world_magnitude_T"] = f"{delta['world_delta_magnitude_T']:.10e}"
            rows.append(row)
    return rows


def _write_csv(report: dict, path: Path) -> None:
    rows = _flatten_rows(report)
    fieldnames: list[str] = []
    for row in rows:
        for key in row:
            if key not in fieldnames:
                fieldnames.append(key)
    with path.open("w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="wirefield",
        description="Superpose Biot-Savart fields of closed polyline coils at probe points.",
    )
    parser.add_argument("--coils", required=True, help="coil geometry + baseline currents (JSON)")
    parser.add_argument("--probes", required=True, help="probe positions and axes (CSV)")
    parser.add_argument("--scenarios", help="alternative current schemes (JSON, optional)")
    parser.add_argument(
        "--safe-distance",
        type=float,
        required=True,
        help="positive safety distance in metres; points this close to any wire are not computed",
    )
    parser.add_argument("--json-out", required=True, help="write full report as JSON")
    parser.add_argument("--csv-out", help="write flattened report as CSV")
    args = parser.parse_args(argv)

    if args.safe_distance <= 0:
        parser.error("--safe-distance must be positive")

    try:
        coils = load_coils(args.coils)
        probes = load_probes(args.probes)
        scenarios = [baseline_scenario(coils)]
        if args.scenarios:
            scenarios.extend(load_scenarios(args.scenarios, coils))
    except (CoilDataError, ProbeDataError, ScenarioDataError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    report = build_report(coils, probes, scenarios, args.safe_distance)
    Path(args.json_out).write_text(
        json.dumps(report, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )
    if args.csv_out:
        _write_csv(report, Path(args.csv_out))
    print(f"wrote {args.json_out}" + (f" and {args.csv_out}" if args.csv_out else ""))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
