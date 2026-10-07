"""Command line interface for the series bar plasticity tool."""

from __future__ import annotations

import argparse
import sys

from .report import build_diagnostics, write_diagnostics_json, write_results_csv
from .runner import RunConfig, load_displacements, load_model, run_history


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="series-bar",
        description="Cyclic plasticity analysis of a 1D series bar (stdlib only).",
    )
    parser.add_argument("config", help="JSON file with element geometry and material")
    parser.add_argument("displacements", help="CSV file with prescribed end displacement per step")
    parser.add_argument("--results", default="results.csv", help="output CSV path")
    parser.add_argument("--diagnostics", default="diagnostics.json", help="output JSON path")
    parser.add_argument("--tol-force", type=float, default=RunConfig.tol_force)
    parser.add_argument("--tol-disp", type=float, default=RunConfig.tol_disp)
    parser.add_argument("--max-iter", type=int, default=RunConfig.max_iter)
    args = parser.parse_args(argv)

    config = RunConfig(
        tol_force=args.tol_force, tol_disp=args.tol_disp, max_iter=args.max_iter
    )
    if config.tol_force <= 0 or config.tol_disp <= 0 or config.max_iter < 1:
        print("error: tolerances must be positive and max-iter >= 1", file=sys.stderr)
        return 2

    try:
        model = load_model(args.config)
        displacements = load_displacements(args.displacements)
    except (OSError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    results, failed = run_history(model, displacements, config)
    write_results_csv(args.results, results)
    diagnostics = build_diagnostics(
        results, failed, config.tol_force, config.tol_disp, config.max_iter
    )
    write_diagnostics_json(args.diagnostics, diagnostics)

    if failed is not None:
        print(
            f"FAILED at step {failed.step} after {failed.iterations} iterations "
            f"(force imbalance {failed.force_imbalance:.6g}); "
            f"{len(results)} converged step(s) written.",
            file=sys.stderr,
        )
        return 1
    print(f"OK: {len(results)} step(s) converged. "
          f"Results: {args.results}, diagnostics: {args.diagnostics}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
