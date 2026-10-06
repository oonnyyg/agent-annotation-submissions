"""CLI entry point: python -m gear_kinematics INPUT.json [-o CSV] [-d JSON].

Reads a gear-train definition plus operating cases, writes shaft speeds as
CSV and constraint diagnostics as JSON.
"""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .model import ModelError, load_train
from .power import power_balance
from .solver import residuals, solve


def run(data: dict):
    train = load_train(data)
    csv_rows = [["case", "shaft", "speed_rpm"]]
    diagnostics = {"cases": []}
    for case in train.cases:
        sol = solve(train, case)
        entry = {"name": case.name, "status": sol.status}
        if sol.status == "unique":
            entry["residuals"] = residuals(train, case, sol.speeds)
            entry["power"] = power_balance(train, case, sol.speeds)
            for shaft in train.shafts:
                csv_rows.append([case.name, shaft, f"{sol.speeds[shaft]:.10g}"])
        elif sol.status == "underdetermined":
            entry["degrees_of_freedom"] = len(sol.free_shafts)
            entry["free_shafts"] = sol.free_shafts
            for shaft in train.shafts:
                csv_rows.append([case.name, shaft, ""])
        else:
            entry["conflict_sources"] = sol.conflict_sources
            for shaft in train.shafts:
                csv_rows.append([case.name, shaft, ""])
        diagnostics["cases"].append(entry)
    return csv_rows, diagnostics


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(prog="gear_kinematics")
    parser.add_argument("input", help="JSON gear train and cases file")
    parser.add_argument("-o", "--speeds", default="speeds.csv",
                        help="output CSV of shaft speeds (default: speeds.csv)")
    parser.add_argument("-d", "--diagnostics", default="diagnostics.json",
                        help="diagnostics JSON (default: diagnostics.json)")
    args = parser.parse_args(argv)

    with open(args.input, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    try:
        csv_rows, diagnostics = run(data)
    except ModelError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    with open(args.speeds, "w", encoding="utf-8", newline="") as fh:
        csv.writer(fh).writerows(csv_rows)
    with open(args.diagnostics, "w", encoding="utf-8") as fh:
        json.dump(diagnostics, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
    return 0


if __name__ == "__main__":
    sys.exit(main())
