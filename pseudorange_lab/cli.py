"""Command line interface for the pseudorange position lab."""
from __future__ import annotations

import argparse
import sys

from .data import load_initial_json, load_observations_csv
from .errors import DataError
from .report import process_epochs, write_csv_report, write_json_report


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="pseudorange-lab",
        description=(
            "Offline weighted least-squares solver for teaching "
            "pseudorange data (no real navigation signals)."
        ),
    )
    parser.add_argument("observations_csv", help="observation CSV file")
    parser.add_argument("initial_json", help="per-epoch initial values JSON")
    parser.add_argument("--json-out", default="report.json",
                        help="JSON report path (default: report.json)")
    parser.add_argument("--csv-out", default="report.csv",
                        help="CSV report path (default: report.csv)")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        epochs = load_observations_csv(args.observations_csv)
        load_initial_json(args.initial_json, epochs)
    except DataError as exc:
        print(f"input error: {exc}", file=sys.stderr)
        return 2
    report = process_epochs(epochs)
    write_json_report(report, args.json_out)
    write_csv_report(report, args.csv_out)
    ok = sum(1 for e in report["epochs"] if e["status"] == "ok")
    total = len(report["epochs"])
    print(f"solved {ok}/{total} epochs; reports: {args.json_out}, {args.csv_out}")
    return 0 if ok == total else 1


if __name__ == "__main__":
    raise SystemExit(main())
