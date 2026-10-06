"""Command line interface: python -m gear_kinematics input.json [-o outdir].

Reads a gear-train document, solves every case independently, writes
speeds.csv and diagnostics.json. Power balance is reported only for cases
with a unique speed solution.
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import os
import sys

from .errors import InputError
from .model import load_document
from .solver import solve, residuals, TOL

RPM_TO_RAD_S = 2.0 * math.pi / 60.0


def run_case(train, case):
    constraints = train.constraints + case.inputs
    result = solve(constraints, train.axes)
    diag = {
        "case": case.name,
        "status": result["status"],
        "constraints_total": len(constraints),
    }
    speeds = {}  # axis -> rpm or None

    if result["status"] == "conflicting":
        diag["conflict_sources"] = result["conflict_sources"]
        diag["message"] = "contradictory constraints; no speed solution"
    else:
        diag["dof"] = result["dof"]
        diag["free_axes"] = result["free_axes"]
        for axis in train.axes:
            speeds[axis] = result["determined"].get(axis)
        sol = result.get("solution")
        if sol is None:
            diag["message"] = "underdetermined; %d remaining dof" % result["dof"]
        else:
            diag["residuals"] = residuals(constraints, train.axes, sol)
            diag["max_abs_residual"] = max(
                (abs(r["residual"]) for r in diag["residuals"]), default=0.0)

    if result["status"] == "unique":
        sol = result["solution"]
        per_axis = {}
        total = 0.0
        for i, axis in enumerate(train.axes):
            torque = case.torques.get(axis, 0.0)
            power = torque * sol[i] * RPM_TO_RAD_S
            per_axis[axis] = {"torque_Nm": torque, "power_W": power}
            total += power
        diag["power"] = {"per_axis": per_axis, "total_W": total,
                         "balanced": abs(total) <= 1e-6}
    else:
        diag["power"] = None  # never emit pseudo-certain power
    return speeds, diag


def main(argv=None):
    parser = argparse.ArgumentParser(prog="gear_kinematics")
    parser.add_argument("input", help="gear-train JSON document")
    parser.add_argument("-o", "--outdir", default=".",
                        help="output directory (default: current)")
    parser.add_argument("--speeds", default="speeds.csv")
    parser.add_argument("--diagnostics", default="diagnostics.json")
    args = parser.parse_args(argv)

    with open(args.input, "r", encoding="utf-8") as fh:
        doc = json.load(fh)
    try:
        train = load_document(doc)
    except InputError as exc:
        print(f"input error: {exc}", file=sys.stderr)
        return 2

    rows = []
    diagnostics = {"tolerance": TOL, "cases": []}
    for case in train.cases:
        speeds, diag = run_case(train, case)
        diagnostics["cases"].append(diag)
        for axis in train.axes:
            rpm = speeds.get(axis)
            rows.append({"case": case.name, "axis": axis,
                         "rpm": "" if rpm is None else f"{rpm:.9g}"})

    os.makedirs(args.outdir, exist_ok=True)
    speeds_path = os.path.join(args.outdir, args.speeds)
    diag_path = os.path.join(args.outdir, args.diagnostics)
    with open(speeds_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=["case", "axis", "rpm"])
        writer.writeheader()
        writer.writerows(rows)
    with open(diag_path, "w", encoding="utf-8") as fh:
        json.dump(diagnostics, fh, indent=2, ensure_ascii=False)
    print(f"wrote {speeds_path} and {diag_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
