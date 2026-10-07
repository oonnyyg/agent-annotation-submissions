from __future__ import annotations

import argparse
import sys

from .core import InputError, analyze, load_points, load_scales
from .reports import write_reports


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="zero-dimensional-persistence",
        description="Compute zero-dimensional connectivity persistence for 2D points.",
    )
    parser.add_argument("points_csv", help="CSV with id, x, and y columns")
    parser.add_argument("scales_json", help="JSON list or {\"scales\": [...]} query thresholds")
    parser.add_argument(
        "-o",
        "--output-dir",
        default="persistence_report",
        help="directory for JSON and CSV reports (default: persistence_report)",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        points = load_points(args.points_csv)
        scales = load_scales(args.scales_json)
        result = analyze(points, scales)
        write_reports(result, args.output_dir)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
