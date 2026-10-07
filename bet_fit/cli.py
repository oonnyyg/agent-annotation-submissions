"""Command-line interface: python -m bet_fit <data.csv> <config.json> [-o out]."""

from __future__ import annotations

import argparse
import sys

from .data import DataError, load_config, load_samples
from .report import render_report


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="bet_fit",
        description="BET specific surface area analysis with automatic "
                    "fit-interval selection.")
    parser.add_argument("csv", help="CSV with columns: sample, "
                        "relative_pressure, quantity_mol_per_g")
    parser.add_argument("config", help="JSON config: cross_section_nm2, "
                        "min_points, optional interval [lo, hi]")
    parser.add_argument("-o", "--output", help="write report to this file "
                        "(default: stdout)")
    args = parser.parse_args(argv)

    try:
        config = load_config(args.config)
        samples = load_samples(args.csv)
    except DataError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    report, all_ok = render_report(samples, config)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as fh:
            fh.write(report)
    else:
        sys.stdout.write(report)
    return 0 if all_ok else 1


if __name__ == "__main__":
    raise SystemExit(main())
