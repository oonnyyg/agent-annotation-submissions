"""Command-line interface: read a JSON config, write CSV/JSON outputs."""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .solver import TransportError, simulate


def _write_profile_csv(path, result, cells):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        dx = result["dx"]
        writer.writerow(["time"] + [f"x={(i + 0.5) * dx:.6g}" for i in range(cells)])
        for row in result["profiles"]:
            writer.writerow([row["time"]] + row["concentration"])


def _write_outlet_csv(path, result):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["time", "concentration"])
        for row in result["outlet"]:
            writer.writerow([row["time"], row["concentration"]])


def _write_mass_json(path, result):
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(result["mass_balance"], fh, indent=2)
        fh.write("\n")


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="retarded-transport",
        description="Explicit finite-volume 1-D retarded solute transport.",
    )
    parser.add_argument("config", help="path to the JSON configuration file")
    parser.add_argument("--profile", default="profile.csv", help="profile CSV output")
    parser.add_argument("--outlet", default="outlet.csv", help="outlet curve CSV output")
    parser.add_argument("--mass", default="mass_balance.json", help="mass budget JSON output")
    args = parser.parse_args(argv)

    try:
        with open(args.config, encoding="utf-8") as fh:
            config = json.load(fh)
        result = simulate(config)
    except (OSError, json.JSONDecodeError, TransportError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    _write_profile_csv(args.profile, result, config["cells"])
    _write_outlet_csv(args.outlet, result)
    _write_mass_json(args.mass, result)
    balance = result["mass_balance"]
    print(f"relative mass residual: {balance['relative_residual']:.3e}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
