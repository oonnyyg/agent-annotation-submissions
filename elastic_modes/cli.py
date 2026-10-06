"""Command-line interface: JSON config in, CSV + diagnostics JSON out."""

import argparse
import csv
import json
import sys

from .errors import InputError
from .solver import solve_modes
from .tensor import (
    normalize_direction,
    validate_density,
    validate_orientation,
    validate_stiffness,
)

DEFAULT_REL_TOL = 1e-6


def load_config(path):
    try:
        with open(path, "r", encoding="utf-8") as fh:
            cfg = json.load(fh)
    except OSError as exc:
        raise InputError(f"cannot read config file: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise InputError(f"config is not valid JSON: {exc}") from exc
    if not isinstance(cfg, dict):
        raise InputError("config root must be a JSON object")

    if "density" not in cfg or "stiffness" not in cfg or "directions" not in cfg:
        raise InputError("config requires 'density', 'stiffness' and 'directions'")
    density = validate_density(cfg["density"])
    stiffness = validate_stiffness(cfg["stiffness"])
    orientation = validate_orientation(cfg.get("orientation", _identity()))

    rel_tol = cfg.get("degeneracy_rel_tol", DEFAULT_REL_TOL)
    if not isinstance(rel_tol, (int, float)) or isinstance(rel_tol, bool) or rel_tol <= 0:
        raise InputError("degeneracy_rel_tol must be a positive number")

    directions = cfg["directions"]
    if not isinstance(directions, list) or not directions:
        raise InputError("directions must be a non-empty list of 3-vectors")
    directions = [normalize_direction(d) for d in directions]
    return density, stiffness, orientation, float(rel_tol), directions


def _identity():
    return [[1.0, 0.0, 0.0], [0.0, 1.0, 0.0], [0.0, 0.0, 1.0]]


def write_csv(path, results):
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "direction_x", "direction_y", "direction_z",
                "mode_rank", "degeneracy", "velocity_m_per_s",
                "eigenvalue_pa", "polarization_x", "polarization_y",
                "polarization_z", "eigen_residual",
            ]
        )
        for res in results:
            d = res["direction"]
            for rank, mode in enumerate(res["modes"], start=1):
                pol = mode.get("polarization")
                row = [
                    *[f"{v:.12g}" for v in d],
                    rank,
                    mode["degeneracy"],
                    f"{mode['velocity_m_per_s']:.12g}",
                    f"{mode['eigenvalue']:.12g}",
                ]
                if pol is not None:
                    row += [f"{v:.12g}" for v in pol]
                else:
                    row += ["", "", ""]
                row.append(f"{mode['eigen_residual']:.6g}")
                writer.writerow(row)


def write_json(path, results, rel_tol):
    payload = {"degeneracy_rel_tol": rel_tol, "directions": results}
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=2)
        fh.write("\n")


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="python -m elastic_modes",
        description="Elastic wave mode solver for anisotropic materials.",
    )
    parser.add_argument("config", help="JSON config: density, stiffness, orientation, directions")
    parser.add_argument("--csv", default="velocities.csv", help="output CSV path")
    parser.add_argument("--json", default="diagnostics.json", help="output diagnostics JSON path")
    args = parser.parse_args(argv)

    try:
        density, stiffness, orientation, rel_tol, directions = load_config(args.config)
        results = [
            solve_modes(stiffness, density, d, orientation, rel_tol) for d in directions
        ]
        write_csv(args.csv, results)
        write_json(args.json, results, rel_tol)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    print(f"wrote {args.csv} and {args.json}")
    return 0
