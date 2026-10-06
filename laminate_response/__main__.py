"""Command line interface: python -m laminate_response input.json [-o out.json] [--csv out.csv]."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .clt import SingularSystemError, analyze_laminate, assemble_abd
from .material import InputError, Laminate, Layer, Material


def load_definition(path: str) -> tuple[Laminate, list[dict]]:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    materials = {}
    for name, props in data.get("materials", {}).items():
        materials[name] = Material(
            name=name,
            e1=float(props["e1"]),
            e2=float(props["e2"]),
            nu12=float(props["nu12"]),
            g12=float(props["g12"]),
            alpha1=float(props["alpha1"]),
            alpha2=float(props["alpha2"]),
        )
    layers = []
    for entry in data.get("layup", []):
        mat_name = entry["material"]
        if mat_name not in materials:
            raise InputError(f"unknown material {mat_name!r} in layup")
        layers.append(
            Layer(
                material=materials[mat_name],
                thickness=float(entry["thickness"]),
                angle_deg=float(entry["angle_deg"]),
            )
        )
    laminate = Laminate(layers=tuple(layers))
    laminate.validate()
    return laminate, list(data.get("load_cases", []))


def write_csv(path: str, cases: list[dict]) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "case",
                "layer",
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
        for case in cases:
            for layer in case["layers"]:
                for surf in layer["surfaces"]:
                    sg = surf["stress_global"]
                    sm = surf["stress_material"]
                    writer.writerow(
                        [case["name"], layer["layer_index"], surf["surface"], surf["z"]]
                        + sg
                        + sm
                    )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="laminate_response")
    parser.add_argument("input", help="input JSON file")
    parser.add_argument("-o", "--output", help="output JSON file (default: stdout)")
    parser.add_argument("--csv", help="output CSV file for layer surface stresses")
    args = parser.parse_args(argv)
    try:
        laminate, load_cases = load_definition(args.input)
        abd, n_th, m_th = assemble_abd(laminate)
        cases = analyze_laminate(laminate, load_cases)
    except (InputError, SingularSystemError, KeyError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    result = {
        "abd": abd,
        "thermal_forces_per_delta_t": n_th,
        "thermal_moments_per_delta_t": m_th,
        "total_thickness": laminate.total_thickness,
        "cases": cases,
    }
    text = json.dumps(result, indent=2)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as fh:
            fh.write(text + "\n")
    else:
        print(text)
    if args.csv:
        write_csv(args.csv, cases)
    return 0


if __name__ == "__main__":
    sys.exit(main())
