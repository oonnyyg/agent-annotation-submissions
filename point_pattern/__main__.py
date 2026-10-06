"""Command-line entry point: python -m point_pattern POINTS_CSV CONFIG_JSON."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .core import PatternError, compute_k, compute_l, simulate_envelope
from .io_utils import (
    build_pattern_and_grid,
    load_config,
    load_points_csv,
    write_curves_csv,
    write_test_json,
)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m point_pattern",
        description=(
            "Ripley K/L analysis with translation edge correction and a "
            "Monte Carlo CSR test for points in a rectangle. No plots are "
            "generated; outputs are CSV and JSON."
        ),
    )
    parser.add_argument("points_csv", type=Path, help="CSV with x,y columns")
    parser.add_argument("config_json", type=Path, help="JSON configuration")
    parser.add_argument(
        "--out-dir",
        type=Path,
        default=Path("."),
        help="directory for k_l_curves.csv and simulation_test.json",
    )
    args = parser.parse_args(argv)

    try:
        points = load_points_csv(args.points_csv)
        config = load_config(args.config_json)
        pattern, grid, n_sim, seed = build_pattern_and_grid(config, points)

        k_result = compute_k(
            pattern.width, pattern.height, pattern.points, grid.radii
        )
        l_values = compute_l(k_result["k_corrected"])
        sim_result = simulate_envelope(
            pattern.width,
            pattern.height,
            pattern.n,
            grid.radii,
            n_sim,
            seed,
            l_values,
        )

        args.out_dir.mkdir(parents=True, exist_ok=True)
        curves_path = args.out_dir / "k_l_curves.csv"
        test_path = args.out_dir / "simulation_test.json"
        write_curves_csv(
            curves_path,
            grid.radii,
            k_result["count"],
            k_result["k_uncorrected"],
            k_result["k_corrected"],
            l_values,
        )
        write_test_json(test_path, sim_result, grid.radii)
    except PatternError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    print(f"wrote {curves_path}")
    print(f"wrote {test_path}")
    print(
        f"max|L-r| = {sim_result['statistic']:.6g}, "
        f"Monte Carlo p = {sim_result['p_value']:.6g} "
        f"({sim_result['n_simulations']} simulations, seed {sim_result['seed']})"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
