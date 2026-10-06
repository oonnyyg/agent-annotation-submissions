"""Command-line interface for the closed-network MVA tool."""

from __future__ import annotations

import argparse
import sys

from .model import ConfigError, load_config
from .mva import run_scenarios
from .report import write_csv, write_json


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="closed-network-mva",
        description="Mean value analysis of a single-class closed queueing network.")
    parser.add_argument("config", help="path to the JSON network configuration")
    parser.add_argument("--csv", default="mva_details.csv",
                        help="output CSV path (default: mva_details.csv)")
    parser.add_argument("--json", default="mva_summary.json",
                        help="output JSON summary path (default: mva_summary.json)")
    args = parser.parse_args(argv)

    try:
        config = load_config(args.config)
        results = run_scenarios(config)
        write_csv(results, args.csv)
        write_json(config, results, args.json)
    except ConfigError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    for result in results:
        rel = (f", {result.relative_throughput:.4f}x baseline"
               if result.relative_throughput is not None else "")
        print(f"scenario '{result.name}': N={result.population}, "
              f"throughput={result.throughput:.6g}, "
              f"cycle_time={result.cycle_time:.6g}{rel}")
    print(f"wrote {args.csv} and {args.json}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
