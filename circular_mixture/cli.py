"""Command-line interface for the circular mixture fitter."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys
from pathlib import Path

from .em import Component, fit
from .io import InputError, load_config, load_observations, to_radians


def _sorted_components(
    components: list[Component],
) -> tuple[list[Component], list[int]]:
    """Sort components by normalized direction; returns (sorted, order)."""
    order = sorted(range(len(components)), key=lambda j: components[j].mu)
    return [components[j] for j in order], order


def write_assignments(
    path: str | Path,
    angles: list[float],
    weights: list[float],
    resp: list[list[float]],
    order: list[int],
    unit: str,
) -> None:
    """Per-observation CSV: angle (declared unit + radians), weight, one
    responsibility column per sorted component, and the MAP assignment."""
    k = len(order)
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        header = [f"angle_{unit}", "angle_radians", "weight"]
        header += [f"resp_component_{j + 1}" for j in range(k)]
        header += ["assigned_component"]
        writer.writerow(header)
        scale = math.pi / 180.0 if unit == "degrees" else 1.0
        for angle, w, r in zip(angles, weights, resp):
            probs = [r[j] for j in order]
            best = probs.index(max(probs)) + 1
            writer.writerow(
                [repr(angle), repr(angle * scale % (2.0 * math.pi)), repr(w)]
                + [repr(p) for p in probs]
                + [best]
            )


def build_report(result, order: list[int], unit: str) -> dict:
    components = [result.components[j] for j in order]
    return {
        "converged": result.converged,
        "status": "converged" if result.converged else "NOT CONVERGED",
        "message": result.message,
        "iterations": result.iterations,
        "log_likelihood": result.log_likelihood,
        "log_likelihood_history": result.log_likelihood_history,
        "unit": unit,
        "components": [
            {
                "weight": c.weight,
                "mu_radians": c.mu,
                "mu_degrees": math.degrees(c.mu),
                "kappa": c.kappa,
            }
            for c in components
        ],
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="circular-mixture-fit",
        description="Fit a 1-3 component von Mises mixture to weighted "
        "circular observations via weighted EM.",
    )
    parser.add_argument("observations", help="CSV with 'angle' and 'weight' columns")
    parser.add_argument("config", help="JSON config (unit, components, tol, max_iter)")
    parser.add_argument("--assignments", default="assignments.csv",
                        help="output CSV of per-observation responsibilities")
    parser.add_argument("--report", default="report.json",
                        help="output JSON convergence report")
    args = parser.parse_args(argv)

    try:
        raw_angles, weights = load_observations(args.observations)
        cfg = load_config(args.config)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    angles = to_radians(raw_angles, cfg["unit"])
    result = fit(
        angles,
        weights,
        cfg["components"],
        tol=cfg["tolerance"],
        max_iter=cfg["max_iterations"],
    )

    _, order = _sorted_components(result.components)
    write_assignments(args.assignments, raw_angles, weights,
                      result.responsibilities, order, cfg["unit"])
    report = build_report(result, order, cfg["unit"])
    with open(args.report, "w", encoding="utf-8") as fh:
        json.dump(report, fh, indent=2)
        fh.write("\n")

    if not result.converged:
        print(f"warning: {result.message}", file=sys.stderr)
        return 1
    print(f"converged in {result.iterations} iterations; "
          f"log-likelihood {result.log_likelihood:.6f}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
