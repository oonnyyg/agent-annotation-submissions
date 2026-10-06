"""Command line entry point: python -m catenary_clearance input.json"""

import argparse
import csv
import json
import sys
from pathlib import Path

from .analysis import run, sample_rows


def _safe_name(name):
    return "".join(c if c.isalnum() or c in "-_" else "_" for c in name)


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="catenary_clearance",
        description="Catenary shape, tension and terrain-clearance checker.",
    )
    parser.add_argument("input", help="path to the JSON configuration file")
    parser.add_argument(
        "--out-dir", default=None,
        help="directory for report.json and sample CSVs "
             "(default: alongside the input file)",
    )
    parser.add_argument(
        "--samples", type=int, default=None,
        help="override the number of CSV sample rows (extrema are unaffected)",
    )
    args = parser.parse_args(argv)

    try:
        with open(args.input, "r", encoding="utf-8") as fh:
            config = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2

    if args.samples is not None:
        config["samples"] = args.samples

    try:
        report, solutions, ctx = run(config)
    except ValueError as exc:
        print(f"error: invalid input: {exc}", file=sys.stderr)
        return 2

    out_dir = Path(args.out_dir) if args.out_dir else Path(args.input).resolve().parent
    out_dir.mkdir(parents=True, exist_ok=True)

    report_path = out_dir / "report.json"
    with open(report_path, "w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)
        fh.write("\n")

    for case in report["cases"]:
        name = case["name"]
        if name not in solutions:
            continue
        csv_path = out_dir / f"samples_{_safe_name(name)}.csv"
        with open(csv_path, "w", encoding="utf-8", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(["x", "cable_y", "terrain_y", "clearance"])
            for row in sample_rows(solutions[name], ctx["terrain"], ctx["samples"]):
                writer.writerow([f"{v:.10g}" for v in row])

    for case in report["cases"]:
        if not case["solved"]:
            print(f"{case['name']}: FAILED TO SOLVE - {case['error']}")
            continue
        status = "PASS" if case["passed"] else "FAIL"
        clr = case["clearance"]
        extra = " [COLLISION]" if clr["collision"] else ""
        print(
            f"{case['name']}: {status}  H={case['horizontal_tension']:.6g}  "
            f"Tmax={case['max_tension']:.6g}  "
            f"min_clearance={clr['min_clearance']:.6g} @ x={clr['x']:.6g}{extra}"
        )
    print(f"report written to {report_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
