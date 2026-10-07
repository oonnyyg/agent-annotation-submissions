"""Command line interface for the zero-dimensional persistence tool."""

from __future__ import annotations

import argparse
import json
import sys

from io_utils import (
    build_report,
    intervals_csv,
    load_points,
    load_thresholds,
    merges_csv,
)
from persistence import compute_persistence


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Zero-dimensional persistence of a 2D point cloud.")
    parser.add_argument("points", help="CSV file with id,x,y rows")
    parser.add_argument("thresholds", help="JSON file with query thresholds")
    parser.add_argument("--json-out", default="report.json",
                        help="path for the JSON report (default: report.json)")
    parser.add_argument("--csv-prefix", default=None,
                        help="if set, write <prefix>_intervals.csv and "
                             "<prefix>_merges.csv")
    args = parser.parse_args(argv)

    try:
        points = load_points(args.points)
        thresholds = load_thresholds(args.thresholds)
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    result = compute_persistence(points)
    report = build_report(result, thresholds)

    with open(args.json_out, "w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)
        fh.write("\n")
    if args.csv_prefix:
        with open(f"{args.csv_prefix}_intervals.csv", "w",
                  encoding="utf-8", newline="") as fh:
            fh.write(intervals_csv(report))
        with open(f"{args.csv_prefix}_merges.csv", "w",
                  encoding="utf-8", newline="") as fh:
            fh.write(merges_csv(report))

    for query in report["queries"]:
        print(f"threshold {query['threshold']}: "
              f"{query['component_count']} component(s)")
    print(f"report written to {args.json_out}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
