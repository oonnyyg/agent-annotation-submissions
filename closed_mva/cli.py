"""Command-line interface for the closed-network MVA tool."""

from __future__ import annotations

import argparse
import sys

from .analysis import ConsistencyError, analyze
from .model import InputError, load_network
from .report import write_csv, write_json


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="closed-mva",
        description="Mean value analysis of a single-class closed queueing network.")
    parser.add_argument("input", help="path to the JSON network definition")
    parser.add_argument("--csv", default="mva_detail.csv",
                        help="CSV detail output path (default: %(default)s)")
    parser.add_argument("--json", default="mva_summary.json",
                        help="JSON summary output path (default: %(default)s)")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        net = load_network(args.input)
        report = analyze(net)
    except InputError as exc:
        print(f"input error: {exc}", file=sys.stderr)
        return 2
    except ConsistencyError as exc:
        print(f"consistency error: {exc}", file=sys.stderr)
        return 3
    write_csv(report, args.csv)
    write_json(report, args.json)
    print(f"visit ratios (reference {report.reference_station!r} = 1):")
    for name, v in report.visit_ratios.items():
        print(f"  {name}: {v:.6g}")
    for sc in report.scenarios:
        change = sc.throughput_change_vs_baseline
        delta = "baseline" if change is None else f"{change:+.2%} vs baseline"
        print(f"scenario {sc.name!r}: N={sc.population} "
              f"X0={sc.final.throughput:.6g} cycle={sc.final.cycle_time:.6g} ({delta})")
    print(f"wrote {args.csv} and {args.json}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
