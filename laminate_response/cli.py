"""Command line interface: JSON in, JSON + per-case CSV out."""

from __future__ import annotations

import argparse
import csv
import json
import os
import sys

from .model import InputError, load_document
from .solver import SingularSystemError, solve_case


def _result_to_dict(result):
    return {
        "case": result.name,
        "midplane_strain": {
            "eps_x0": result.midplane_strain[0],
            "eps_y0": result.midplane_strain[1],
            "gamma_xy0": result.midplane_strain[2],
        },
        "curvature": {
            "kappa_x": result.curvature[0],
            "kappa_y": result.curvature[1],
            "kappa_xy": result.curvature[2],
        },
        "ABD": {"A": result.a, "B": result.b, "D": result.d},
        "thermal_resultants": {
            "NT": result.thermal_forces,
            "MT": result.thermal_moments,
        },
        "equilibrium_residual": {
            "forces": result.residual_forces,
            "moments": result.residual_moments,
        },
    }


def _write_csv(result, path):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "ply_index",
                "surface",
                "z",
                "sigma_x",
                "sigma_y",
                "tau_xy",
                "sigma_1",
                "sigma_2",
                "tau_12",
            ]
        )
        for s in result.surface_stresses:
            writer.writerow(
                [
                    s.ply_index,
                    s.surface,
                    f"{s.z:.10g}",
                    f"{s.sigma_x:.10g}",
                    f"{s.sigma_y:.10g}",
                    f"{s.tau_xy:.10g}",
                    f"{s.sigma_1:.10g}",
                    f"{s.sigma_2:.10g}",
                    f"{s.tau_12:.10g}",
                ]
            )


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="python -m laminate_response",
        description="Classical laminate theory analysis with thermal loading.",
    )
    parser.add_argument("input", help="input JSON document")
    parser.add_argument(
        "-o",
        "--output",
        default="results.json",
        help="overall results JSON path (default: results.json)",
    )
    parser.add_argument(
        "--csv-dir",
        default=".",
        help="directory for per-case ply stress CSV files",
    )
    args = parser.parse_args(argv)

    try:
        materials, laminate, cases = load_document(args.input)
    except (InputError, OSError, json.JSONDecodeError) as exc:
        print(f"input error: {exc}", file=sys.stderr)
        return 2

    os.makedirs(args.csv_dir, exist_ok=True)
    summary = {
        "input": os.path.abspath(args.input),
        "total_thickness": laminate.total_thickness,
        "ply_count": len(laminate.plies),
        "cases": [],
    }
    for case in cases:
        try:
            result = solve_case(laminate, materials, case)
        except SingularSystemError as exc:
            print(f"case {case.name!r}: {exc}", file=sys.stderr)
            return 3
        entry = _result_to_dict(result)
        csv_name = f"{case.name}_ply_stresses.csv"
        csv_path = os.path.join(args.csv_dir, csv_name)
        _write_csv(result, csv_path)
        entry["ply_stress_csv"] = os.path.abspath(csv_path)
        summary["cases"].append(entry)

    with open(args.output, "w", encoding="utf-8") as fh:
        json.dump(summary, fh, indent=2)
        fh.write("\n")
    print(f"wrote {args.output} and {len(cases)} CSV file(s) to {args.csv_dir}")
    return 0
