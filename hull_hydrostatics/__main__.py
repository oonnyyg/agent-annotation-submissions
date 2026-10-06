"""Command-line interface: python -m hull_hydrostatics <config.json>.

Reads a JSON configuration describing a closed cross-section polygon,
mass per unit length, center of gravity, fluid properties and a list of
heel angles. Writes per-angle results as CSV and geometric diagnostics
as JSON.
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys

from .geometry import GeometryError, area_centroid, validate_polygon
from .solver import analyze_angle

_CSV_FIELDS = [
    "angle_deg",
    "status",
    "translation",
    "buoyancy_x",
    "buoyancy_y",
    "gravity_x",
    "gravity_y",
    "displaced_mass",
    "buoyancy",
    "weight",
    "residual",
    "moment_about_cg",
    "classification",
    "error",
]


class ConfigError(ValueError):
    """Raised for invalid configuration input."""


def _require_positive(config, key):
    value = config.get(key)
    if not isinstance(value, (int, float)) or not math.isfinite(value) or value <= 0:
        raise ConfigError(f"{key!r} must be a positive finite number")
    return float(value)


def load_config(path):
    with open(path, "r", encoding="utf-8") as fh:
        config = json.load(fh)
    if not isinstance(config, dict):
        raise ConfigError("configuration root must be a JSON object")

    section = validate_polygon(config.get("section"))
    mass = _require_positive(config, "mass_per_length")
    rho = _require_positive(config, "fluid_density")
    gravity = _require_positive(config, "gravity")

    cg = config.get("center_of_gravity")
    if (
        not isinstance(cg, (list, tuple))
        or len(cg) != 2
        or not all(isinstance(c, (int, float)) and math.isfinite(c) for c in cg)
    ):
        raise ConfigError("'center_of_gravity' must be [x, y] of finite numbers")
    cg = (float(cg[0]), float(cg[1]))

    angles = config.get("angles_deg")
    if (
        not isinstance(angles, list)
        or not angles
        or not all(isinstance(a, (int, float)) and math.isfinite(a) for a in angles)
    ):
        raise ConfigError("'angles_deg' must be a non-empty list of finite numbers")
    angles = [float(a) for a in angles]

    tol = config.get("moment_tolerance", 1e-9)
    if not isinstance(tol, (int, float)) or not math.isfinite(tol) or tol < 0:
        raise ConfigError("'moment_tolerance' must be a non-negative finite number")

    full_area, section_centroid = area_centroid(section)
    full_displacement_mass = rho * full_area
    if not 0.0 < mass < full_displacement_mass:
        raise ConfigError(
            "mass_per_length must be strictly between 0 and the fully "
            f"submerged displacement mass ({full_displacement_mass!r})"
        )

    return {
        "section": section,
        "mass_per_length": mass,
        "fluid_density": rho,
        "gravity": gravity,
        "center_of_gravity": cg,
        "angles_deg": angles,
        "moment_tolerance": float(tol),
        "full_area": full_area,
        "section_centroid": section_centroid,
    }


def run(config):
    target_area = config["mass_per_length"] / config["fluid_density"]
    results = [
        analyze_angle(
            config["section"],
            config["center_of_gravity"],
            angle,
            target_area,
            config["fluid_density"],
            config["gravity"],
            config["moment_tolerance"],
        )
        for angle in config["angles_deg"]
    ]
    diagnostics = {
        "section_area": config["full_area"],
        "section_centroid": list(config["section_centroid"]),
        "section_vertex_count": len(config["section"]),
        "mass_per_length": config["mass_per_length"],
        "fluid_density": config["fluid_density"],
        "gravity": config["gravity"],
        "full_displacement_mass": config["fluid_density"] * config["full_area"],
        "target_displaced_area": target_area,
        "moment_tolerance": config["moment_tolerance"],
        "angles": results,
    }
    return results, diagnostics


def write_csv(results, path):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=_CSV_FIELDS)
        writer.writeheader()
        for r in results:
            row = {field: r.get(field, "") for field in _CSV_FIELDS}
            if r.get("status") == "ok":
                row["buoyancy_x"], row["buoyancy_y"] = r["buoyancy_center"]
                row["gravity_x"], row["gravity_y"] = r["gravity_center"]
            row.pop("buoyancy_center", None)
            writer.writerow(row)


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="hull_hydrostatics",
        description="Prismatic hull hydrostatic equilibrium and righting moments.",
    )
    parser.add_argument("config", help="path to the JSON configuration file")
    parser.add_argument("--csv", default="results.csv", help="output CSV path")
    parser.add_argument(
        "--diagnostics", default="diagnostics.json", help="output diagnostics JSON path"
    )
    args = parser.parse_args(argv)

    try:
        config = load_config(args.config)
    except (ConfigError, GeometryError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    results, diagnostics = run(config)
    write_csv(results, args.csv)
    with open(args.diagnostics, "w", encoding="utf-8") as fh:
        json.dump(diagnostics, fh, indent=2)
        fh.write("\n")

    failed = [r for r in results if r["status"] != "ok"]
    print(f"wrote {args.csv} and {args.diagnostics}")
    print(f"{len(results) - len(failed)}/{len(results)} angles solved")
    if failed:
        for r in failed:
            print(f"  angle {r['angle_deg']}: failed: {r['error']}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
