"""Command-line interface for the raking tool."""

from __future__ import annotations

import argparse
import sys

from .core import STATUS_CONVERGED, rake
from .errors import RakingError
from .inputs import load_config, load_samples
from .report import write_diagnostics, write_success


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="raking",
        description="Adjust survey sample weights to known population "
                    "margins via iterative proportional fitting.")
    parser.add_argument("samples", help="CSV with id, weight, categorical fields")
    parser.add_argument("targets", help="JSON with margins, tolerance, "
                                        "max_iterations")
    parser.add_argument("--out-dir", default="raking_out",
                        help="output directory (default: raking_out)")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        samples, field_names = load_samples(args.samples)
        config = load_config(args.targets, samples, field_names)
    except RakingError as exc:
        print(f"input error: {exc}", file=sys.stderr)
        return 2

    result = rake(samples, config.margins, config.tolerance,
                  config.max_iterations)
    if result.status == STATUS_CONVERGED:
        paths = write_success(args.out_dir, samples, config.margins, result)
        print(f"converged in {result.iterations} iteration(s)")
        print(f"weights: {paths['weights']}")
        print(f"margins: {paths['margins']}")
        return 0

    path = write_diagnostics(args.out_dir, result)
    print(f"NOT converged (status={result.status}); "
          f"diagnostics written to {path}", file=sys.stderr)
    for failure in result.failures:
        print(f"  failure: margin={failure.margin} "
              f"category={failure.category}: {failure.reason}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
