"""Command-line interface for the Green-Ampt infiltration calculator."""

from __future__ import annotations

import argparse
import sys

from .io import load_segments, load_soil
from .model import simulate


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="green-ampt",
        description="Replay piecewise-constant rainfall through the "
                    "Green-Ampt infiltration model.",
    )
    parser.add_argument("rainfall", help="CSV file with columns start,end,rate")
    parser.add_argument("soil", help="JSON file with ks, psi, delta_theta")
    parser.add_argument(
        "--tol", type=float, default=1e-9,
        help="solver tolerance on cumulative infiltration in mm "
             "(default: 1e-9)",
    )
    parser.add_argument(
        "--max-iter", type=int, default=200,
        help="maximum solver iterations (default: 200)",
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        params = load_soil(args.soil)
        segments = load_segments(args.rainfall)
        result = simulate(params, segments, tol=args.tol, max_iter=args.max_iter)
    except (ValueError, RuntimeError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print("time_h      kind          cum_rain_mm  cum_infil_mm  "
          "cum_runoff_mm  infil_rate_mm_h")
    for ev in result.events:
        print(f"{ev.time:10.6f}  {ev.kind:<13} {ev.cum_rain:12.6f} "
              f"{ev.cum_infiltration:13.6f} {ev.cum_runoff:14.6f} "
              f"{ev.infil_rate:16.6f}")
    print()
    print(f"total rainfall     : {result.total_rain:.6f} mm")
    print(f"total infiltration : {result.total_infiltration:.6f} mm")
    print(f"total runoff       : {result.total_runoff:.6f} mm")
    balance = result.total_rain - result.total_infiltration - result.total_runoff
    print(f"mass balance error : {balance:.3e} mm")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())