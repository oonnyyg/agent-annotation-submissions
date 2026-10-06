"""Command-line interface for the transit exposure model.

Usage:
    python transit_cli.py config.json exposures.csv out.csv diagnostics.json

config.json keys:
    planet_radius_ratio   (0, 1]
    impact_parameter      >= 0
    t_mid_hours           finite
    velocity              > 0, stellar radii per hour
    limb_darkening        [u1, u2], u1, u2 >= 0, u1 + u2 <= 1
    tolerance             > 0 (optional, default 1e-10)
    max_depth             positive int (optional, default 30)

exposures.csv columns: center_hours, duration_hours (header required).
"""

from __future__ import annotations

import csv
import json
import sys

from transit_model import TransitModel


def load_config(path):
    with open(path, "r", encoding="utf-8") as handle:
        cfg = json.load(handle)
    u1, u2 = cfg.get("limb_darkening", [0.0, 0.0])
    model = TransitModel(
        planet_radius=cfg["planet_radius_ratio"],
        impact_parameter=cfg["impact_parameter"],
        t_mid_hours=cfg["t_mid_hours"],
        velocity=cfg["velocity"],
        u1=u1,
        u2=u2,
        tolerance=cfg.get("tolerance", 1e-10),
        max_depth=int(cfg.get("max_depth", 30)),
    )
    return model


def load_exposures(path):
    rows = []
    with open(path, "r", encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle)
        for line, row in enumerate(reader, start=2):
            try:
                center = float(row["center_hours"])
                duration = float(row["duration_hours"])
            except (TypeError, ValueError) as exc:
                raise ValueError(f"{path}:{line}: bad numeric row: {row!r}") from exc
            rows.append((center, duration))
    return rows


def main(argv):
    if len(argv) != 5:
        print(__doc__.strip(), file=sys.stderr)
        return 2
    _, config_path, exposures_path, out_csv_path, out_json_path = argv

    model = load_config(config_path)
    exposures = load_exposures(exposures_path)
    model.diagnostics["converged"] = True

    rows = []
    for center, duration in exposures:
        inst = model.instantaneous(center)
        mean = model.exposure_mean(center, duration)
        rows.append(
            {
                "center_hours": center,
                "duration_hours": duration,
                "instantaneous": inst.value,
                "exposure_mean": mean.value,
                "converged": inst.converged and mean.converged,
            }
        )

    with open(out_csv_path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.DictWriter(handle, fieldnames=list(rows[0].keys()) if rows else
                                ["center_hours", "duration_hours",
                                 "instantaneous", "exposure_mean", "converged"])
        writer.writeheader()
        writer.writerows(rows)

    diagnostics = {
        "total_flux": model.total_flux,
        "contact_interval_hours": model.contact_interval(),
        "contact_times_hours": model.contact_times(),
        "converged": model.diagnostics.get("converged", True),
        "blocked_flux_calls": model.diagnostics.get("blocked_flux_calls", 0),
        "blocked_flux_evaluations": model.diagnostics.get(
            "blocked_flux_evaluations", 0
        ),
        "tolerance": model.tolerance,
    }
    with open(out_json_path, "w", encoding="utf-8") as handle:
        json.dump(diagnostics, handle, indent=2)
        handle.write("\n")

    if not diagnostics["converged"]:
        print("WARNING: numerical integration did not fully converge",
              file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
