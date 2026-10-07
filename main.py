"""Command-line interface for the Green-Ampt infiltration calculator.

Usage:
    python main.py RAIN.csv SOIL.json [--tol 1e-9] [--max-iter 100]

RAIN.csv columns (with header): t_start,t_end,intensity
    Times in hours, intensity in mm/h. Segments must be contiguous.
SOIL.json keys: ks, psi, delta_theta
    Saturated conductivity [mm/h], wetting-front suction [mm],
    initial moisture deficit [-].
"""

import argparse
import csv
import json
import sys

from greenampt import (Segment, SoilParams, SolverError, simulate, totals)


def load_segments(path):
    segments = []
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.DictReader(fh)
        required = {"t_start", "t_end", "intensity"}
        if not required <= set(reader.fieldnames or []):
            raise ValueError(
                f"{path}: CSV header must contain {sorted(required)}")
        for row in reader:
            segments.append(Segment(
                float(row["t_start"]), float(row["t_end"]),
                float(row["intensity"])))
    if not segments:
        raise ValueError(f"{path}: no rainfall segments found")
    return segments


def load_soil(path):
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    try:
        return SoilParams(ks=float(data["ks"]), psi=float(data["psi"]),
                          delta_theta=float(data["delta_theta"]))
    except KeyError as exc:
        raise ValueError(f"{path}: missing soil parameter {exc}") from exc


def format_table(events):
    header = (f"{'time[h]':>10} {'event':>12} {'rain[mm/h]':>10} "
              f"{'inf_rate[mm/h]':>15} {'cum_infil[mm]':>14} "
              f"{'cum_runoff[mm]':>15}")
    lines = [header]
    for e in events:
        lines.append(
            f"{e.t:>10.4f} {e.kind:>12} {e.intensity:>10.3f} "
            f"{e.infiltration_rate:>15.6f} {e.cum_infiltration:>14.6f} "
            f"{e.cum_runoff:>15.6f}")
    return "\n".join(lines)


def main(argv=None):
    parser = argparse.ArgumentParser(
        description="Green-Ampt infiltration calculator for "
                    "piecewise-constant rainfall.")
    parser.add_argument("rainfall", help="CSV file: t_start,t_end,intensity")
    parser.add_argument("soil", help="JSON file: ks, psi, delta_theta")
    parser.add_argument("--tol", type=float, default=1e-9,
                        help="solver tolerance on cumulative time [h]")
    parser.add_argument("--max-iter", type=int, default=100,
                        help="maximum solver iterations")
    args = parser.parse_args(argv)

    try:
        segments = load_segments(args.rainfall)
        soil = load_soil(args.soil)
        events = simulate(segments, soil, tol=args.tol,
                          max_iter=args.max_iter)
    except (ValueError, SolverError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print(format_table(events))
    rain, infil, runoff = totals(segments, events)
    print()
    print(f"total rainfall     : {rain:.6f} mm")
    print(f"total infiltration : {infil:.6f} mm")
    print(f"total runoff       : {runoff:.6f} mm")
    balance = rain - infil - runoff
    print(f"water balance error: {balance:.3e} mm")
    if abs(balance) > 1e-6 * max(1.0, rain):
        print("error: water balance does not close", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
