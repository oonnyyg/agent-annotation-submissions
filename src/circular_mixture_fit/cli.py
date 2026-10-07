"""Command line interface for the circular mixture fitter."""

from __future__ import annotations

import argparse
import sys

from .em import final_responsibilities, fit
from .errors import FitError, InputError
from .io import load_config, load_observations
from .report import build_report, sort_components, write_assignments, write_report


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="circular-mixture-fit",
        description="Fit a weighted von Mises mixture to circular observations.",
    )
    parser.add_argument("--data", required=True, help="CSV with angle,weight columns")
    parser.add_argument("--config", required=True, help="JSON configuration file")
    parser.add_argument(
        "--assignments", required=True, help="output CSV of per-observation membership"
    )
    parser.add_argument("--report", required=True, help="output JSON convergence report")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        cfg = load_config(args.config)
        angles, weights = load_observations(args.data, cfg["unit"])
        result = fit(
            angles,
            weights,
            cfg["components"],
            tol=cfg["tolerance"],
            max_iter=cfg["max_iterations"],
        )
        sorted_components, _order = sort_components(result.components)
        resp, final_ll = final_responsibilities(angles, weights, sorted_components)
        identity = list(range(len(sorted_components)))
        write_assignments(args.assignments, angles, weights, resp, identity, cfg["unit"])
        report = build_report(result, sorted_components, final_ll, cfg["unit"])
        write_report(args.report, report)
    except (InputError, FitError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    if not result.converged:
        print(f"warning: {result.message}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
