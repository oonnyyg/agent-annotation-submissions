"""Command line entry point: python -m gage_study."""

from __future__ import annotations

import argparse
import sys

from .anova import compute_anova
from .data import GageStudyError, load_config, load_measurements
from .report import write_all
from .variance import compute_variance_components


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m gage_study",
        description="Crossed gage R&R study (balanced two-way random effects ANOVA).",
    )
    parser.add_argument("csv", help="measurement CSV file")
    parser.add_argument("config", help="JSON config with column names")
    parser.add_argument(
        "--out-dir",
        default="gage_study_out",
        help="output directory (default: gage_study_out)",
    )
    args = parser.parse_args(argv)

    try:
        config = load_config(args.config)
        data = load_measurements(args.csv, config)
    except (GageStudyError, OSError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    anova = compute_anova(data)
    vc = compute_variance_components(data, anova)
    paths = write_all(args.out_dir, data, anova, vc)

    for warning in vc.warnings:
        print(f"warning: {warning}", file=sys.stderr)
    for name, path in paths.items():
        print(f"{name}: {path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
