"""Command line interface: python -m catenary_clearance config.json -o outdir"""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .analysis import analyze_config
from .solver import CatenaryError, solve_catenary


def _write_samples(report, outdir, samples):
    """Write one sampled CSV per solved scenario.

    Sampling never affects the extrema in the JSON report, which are
    computed analytically.
    """
    span = report["span"]
    weight = report["weight_per_length"]
    written = []
    for scenario in report["scenarios"]:
        if not scenario["ok"]:
            continue
        length = scenario["length"]
        cat = solve_catenary(
            span["x1"], span["y1"], span["x2"], span["y2"], length, weight
        )
        name = f"samples_L{length:g}.csv"
        path = Path(outdir) / name
        n = max(2, samples)
        with path.open("w", newline="", encoding="utf-8") as fh:
            writer = csv.writer(fh)
            writer.writerow(["x", "cable_y", "tension"])
            for i in range(n):
                x = span["x1"] + (span["x2"] - span["x1"]) * i / (n - 1)
                writer.writerow(
                    [f"{x:.10g}", f"{cat.y(x):.10g}", f"{cat.tension_at(x):.10g}"]
                )
        written.append(name)
    return written


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="catenary_clearance",
        description="Catenary shape, tension and terrain clearance checker.",
    )
    parser.add_argument("config", help="path to the JSON input file")
    parser.add_argument(
        "-o", "--outdir", default=".",
        help="output directory (default: current directory)",
    )
    parser.add_argument(
        "-n", "--samples", type=int, default=201,
        help="CSV sampling points per span (default: 201)",
    )
    parser.add_argument(
        "--report", default="report.json",
        help="report file name inside outdir",
    )
    args = parser.parse_args(argv)

    try:
        with open(args.config, "r", encoding="utf-8") as fh:
            config = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read config: {exc}", file=sys.stderr)
        return 2

    try:
        report = analyze_config(config)
    except (CatenaryError, KeyError, TypeError, ValueError) as exc:
        print(f"error: invalid input: {exc}", file=sys.stderr)
        return 2

    outdir = Path(args.outdir)
    outdir.mkdir(parents=True, exist_ok=True)
    csv_files = _write_samples(report, outdir, args.samples)
    report["sample_files"] = csv_files
    report_path = outdir / args.report
    with report_path.open("w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)
        fh.write("\n")

    for scenario in report["scenarios"]:
        if not scenario["ok"]:
            print(f"L={scenario['length']:g}: FAILED - {scenario['error']}")
            continue
        status = "PASS" if scenario["pass"] else "FAIL"
        print(
            f"L={scenario['length']:g}: {status} "
            f"H={scenario['horizontal_tension']:.6g} "
            f"Tmax={scenario['max_tension']:.6g} "
            f"clearance={scenario['clearance']['min']:.6g}"
        )
    print(f"report: {report_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
