"""CLI: python -m hull_hydrostatics config.json --csv out.csv --diag diag.json"""

from __future__ import annotations

import argparse
import csv
import json
import sys

from . import geometry, solver

CSV_FIELDS = [
    "angle_deg", "status", "translation",
    "buoyancy_center_x", "buoyancy_center_y",
    "gravity_center_x", "gravity_center_y",
    "displaced_mass", "submerged_area", "buoyancy",
    "moment_about_cg", "equilibrium_residual", "behaviour", "error",
]


def load_config(path):
    with open(path, "r", encoding="utf-8") as fh:
        cfg = json.load(fh)
    vertices = [tuple(map(float, v)) for v in cfg["polygon"]]
    geometry.validate_simple_polygon(vertices)
    cg = tuple(map(float, cfg["center_of_gravity"]))
    mass = float(cfg["unit_length_mass"])
    rho = float(cfg["fluid_density"])
    g = float(cfg["gravity"])
    if rho <= 0.0 or g <= 0.0:
        raise ValueError("fluid_density and gravity must be positive")
    total = geometry.polygon_area(vertices)
    if not 0.0 < mass < rho * total:
        raise ValueError(
            "unit_length_mass must be strictly between 0 and the fully "
            f"submerged displaced mass ({rho * total!r})")
    angles = [float(a) for a in cfg["angles_deg"]]
    tol = float(cfg.get("moment_neutral_tolerance", 1e-9))
    return vertices, cg, mass, rho, g, angles, tol


def run(cfg_path, csv_path, diag_path):
    vertices, cg, mass, rho, g, angles, tol = load_config(cfg_path)
    rows = []
    diagnostics = {
        "section_area": geometry.polygon_area(vertices),
        "section_centroid": list(geometry.polygon_centroid(vertices)),
        "unit_length_mass": mass,
        "fluid_density": rho,
        "gravity": g,
        "moment_neutral_tolerance": tol,
        "angles": [],
    }
    # Each angle is solved independently; the requested sampling order
    # has no effect on the physics of any individual result.
    for angle in angles:
        try:
            res = solver.analyze_angle(vertices, cg, angle, mass, rho, g, tol)
        except Exception as exc:  # per-angle failure must not abort the run
            res = {"angle_deg": angle, "status": "failed", "error": str(exc)}
        rows.append(res)
        diag_entry = dict(res)
        if res.get("status") == "ok":
            diag_entry["area_residual"] = res["area_residual"]
        diagnostics["angles"].append(diag_entry)

    with open(csv_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=CSV_FIELDS)
        writer.writeheader()
        for r in rows:
            row = {k: "" for k in CSV_FIELDS}
            row["angle_deg"] = r["angle_deg"]
            row["status"] = r["status"]
            if r["status"] == "ok":
                row.update({
                    "translation": r["translation"],
                    "buoyancy_center_x": r["buoyancy_center"][0],
                    "buoyancy_center_y": r["buoyancy_center"][1],
                    "gravity_center_x": r["gravity_center"][0],
                    "gravity_center_y": r["gravity_center"][1],
                    "displaced_mass": r["displaced_mass"],
                    "submerged_area": r["submerged_area"],
                    "buoyancy": r["buoyancy"],
                    "moment_about_cg": r["moment_about_cg"],
                    "equilibrium_residual": r["equilibrium_residual"],
                    "behaviour": r["behaviour"],
                })
            else:
                row["error"] = r["error"]
            writer.writerow(row)

    with open(diag_path, "w", encoding="utf-8") as fh:
        json.dump(diagnostics, fh, indent=2)
        fh.write("\n")
    return rows


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="hull_hydrostatics",
        description="Prismatic hull hydrostatic equilibrium and "
                    "righting-moment analysis.")
    parser.add_argument("config", help="input JSON configuration")
    parser.add_argument("--csv", default="results.csv",
                        help="output CSV path (default: results.csv)")
    parser.add_argument("--diag", default="diagnostics.json",
                        help="output diagnostics JSON (default: diagnostics.json)")
    args = parser.parse_args(argv)
    try:
        rows = run(args.config, args.csv, args.diag)
    except (ValueError, KeyError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    failed = sum(1 for r in rows if r["status"] != "ok")
    print(f"wrote {args.csv} and {args.diag}: "
          f"{len(rows)} angle(s), {failed} failed")
    return 0 if failed == 0 else 1


if __name__ == "__main__":
    sys.exit(main())
