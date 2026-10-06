"""Command line interface for the layered soil storage tool."""

from __future__ import annotations

import argparse
import json
import sys

from .io import dumps_report, load_config, run_config, run_csv_batch
from .model import invert_water_table, profile_storage
from .io import layers_from_config


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(
        prog="soil-storage",
        description="Layered soil water storage under hydrostatic equilibrium "
                    "(lengths in cm; alpha in 1/cm).",
    )
    p.add_argument("config", help="JSON config file with 'layers'")
    p.add_argument("--water-table", type=float, metavar="CM",
                   help="water table depth in cm (overrides config)")
    p.add_argument("--samples", metavar="D1,D2,...", default=None,
                   help="comma-separated profile sample depths in cm")
    p.add_argument("--invert", type=float, metavar="TARGET",
                   help="target total storage in cm to invert for water table")
    p.add_argument("--max-depth", type=float, metavar="CM",
                   help="search upper bound for --invert (cm)")
    p.add_argument("--csv-in", metavar="FILE",
                   help="CSV batch requests (columns: kind,depth_cm)")
    p.add_argument("--csv-out", metavar="FILE",
                   help="write CSV batch results to FILE (default: stdout)")
    return p


def main(argv=None) -> int:
    args = build_parser().parse_args(argv)
    try:
        with open(args.config, "r", encoding="utf-8") as fh:
            config = load_config(fh.read())
        if args.water_table is not None:
            config["water_table"] = args.water_table
        if args.samples is not None:
            config["samples"] = [float(s) for s in args.samples.split(",") if s.strip()]
        if args.invert is not None:
            if args.max_depth is None:
                raise ValueError("--invert requires --max-depth")
            config["invert"] = {"target": args.invert, "max_depth": args.max_depth}

        if args.csv_in:
            with open(args.csv_in, "r", encoding="utf-8", newline="") as fh:
                out_csv = run_csv_batch(config, fh.read())
            if args.csv_out:
                with open(args.csv_out, "w", encoding="utf-8", newline="") as fh:
                    fh.write(out_csv)
            else:
                sys.stdout.write(out_csv)

        report = run_config(config)
        if not args.csv_in or not args.csv_out:
            sys.stdout.write(dumps_report(report))
        inv = report.get("inversion")
        if inv and not inv["solved"]:
            print(f"inversion failed: {inv['message']}", file=sys.stderr)
            return 2
        return 0
    except (ValueError, KeyError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    except RuntimeError as exc:  # integration convergence failure
        print(f"error: {exc}", file=sys.stderr)
        return 3


if __name__ == "__main__":
    sys.exit(main())
