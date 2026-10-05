"""Command-line interface: read a JSON config, write CSV/JSON outputs."""

from __future__ import annotations

import argparse
import csv
import json
import os
import sys

from .model import TransportConfig, run_simulation


def main(argv: list | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="solute-transport",
        description="1-D retarded solute transport with decay (stdlib only).")
    parser.add_argument("config", help="input JSON configuration file")
    parser.add_argument("--profiles", default="profiles.csv",
                        help="output CSV of concentration profiles")
    parser.add_argument("--outlet", default="outlet.csv",
                        help="output CSV of the outlet breakthrough curve")
    parser.add_argument("--mass", default="mass_balance.json",
                        help="output JSON of the mass-balance residuals")
    args = parser.parse_args(argv)

    with open(args.config, "r", encoding="utf-8") as fh:
        cfg = TransportConfig.from_dict(json.load(fh))
    result = run_simulation(cfg)

    for path in (args.profiles, args.outlet, args.mass):
        parent = os.path.dirname(os.path.abspath(path))
        os.makedirs(parent, exist_ok=True)

    centers = result["cell_centers"]
    with open(args.profiles, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["time"] + [f"x={x:.6g}" for x in centers])
        for snap in result["profiles"]:
            writer.writerow([snap["time"]] + snap["concentration"])

    with open(args.outlet, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["time", "concentration"])
        for row in result["outlet"]:
            writer.writerow([row["time"], row["concentration"]])

    with open(args.mass, "w", encoding="utf-8") as fh:
        json.dump(result["mass_balance"], fh, indent=2)
        fh.write("\n")

    mb = result["mass_balance"]
    print(f"cells={cfg.cells} dx={result['dx']:.6g} m  "
          f"stored={mb['stored']:.6g}  residual={mb['residual']:.3e}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
