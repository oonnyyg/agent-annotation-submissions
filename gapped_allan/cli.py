"""Command line interface: read CSV + JSON config, write CSV + JSON reports."""

from __future__ import annotations

import argparse
import sys

from .core import GridError, analyze
from .io import (
    ConfigError,
    load_config,
    load_csv,
    write_csv_report,
    write_json_report,
)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="gapped-allan",
        description=(
            "Overlapping Allan deviation for gapped observations on a "
            "uniform sampling grid. Times in seconds, values dimensionless "
            "fractional frequency."
        ),
    )
    parser.add_argument("csv", help="input CSV with header 'time,value'")
    parser.add_argument("config", help="JSON config: tau0, m list, detrend flag")
    parser.add_argument(
        "--out-prefix",
        default="allan_report",
        help="output prefix; writes <prefix>.csv and <prefix>.json",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        cfg = load_config(args.config)
        times, values = load_csv(args.csv)
        result = analyze(
            times,
            values,
            tau0=cfg["tau0"],
            m_list=cfg["m"],
            detrend=cfg["detrend"],
        )
    except (ConfigError, GridError, ValueError, OSError) as exc:
        # Fail before touching any existing report files.
        print(f"error: {exc}", file=sys.stderr)
        return 1
    csv_path = args.out_prefix + ".csv"
    json_path = args.out_prefix + ".json"
    write_csv_report(result, csv_path)
    write_json_report(result, json_path)
    print(f"wrote {csv_path} and {json_path}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
