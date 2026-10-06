"""Command-line interface: terminal velocities, sampling concentrations,
and mass-balance reports for a still-water settling column."""
from __future__ import annotations

import argparse
import csv
import sys

from .model import (
    ConvergenceError,
    SettlingColumn,
    ValidationError,
    load_scenario,
)


def read_samples(path):
    """Read sampling requests: CSV rows of time,z_top,z_bottom."""
    requests = []
    with open(path, "r", encoding="utf-8", newline="") as handle:
        reader = csv.DictReader(handle)
        required = {"time", "z_top", "z_bottom"}
        if reader.fieldnames is None or not required <= set(reader.fieldnames):
            raise ValidationError(
                "sampling CSV must have columns: time,z_top,z_bottom"
            )
        for line, row in enumerate(reader, start=2):
            try:
                requests.append(
                    (
                        float(row["time"]),
                        float(row["z_top"]),
                        float(row["z_bottom"]),
                    )
                )
            except (TypeError, ValueError):
                raise ValidationError(
                    f"sampling CSV line {line}: non-numeric value"
                ) from None
    if not requests:
        raise ValidationError("sampling CSV contains no requests")
    return requests


def write_velocities(column, path):
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            [
                "class",
                "diameter_m",
                "density_kg_m3",
                "velocity_m_s",
                "reynolds",
                "drag_coefficient",
                "relative_residual",
            ]
        )
        for i, (cls, term) in enumerate(zip(column.classes, column.terminal)):
            writer.writerow(
                [
                    i,
                    f"{cls.diameter:.6e}",
                    f"{cls.density:.6e}",
                    f"{term.velocity:.6e}",
                    f"{term.reynolds:.6e}",
                    f"{term.drag_coefficient:.6e}",
                    f"{term.relative_residual:.3e}",
                ]
            )


def write_concentrations(column, requests, path):
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        header = ["time_s", "z_top_m", "z_bottom_m"] + [
            f"conc_class_{i}_kg_m3" for i in range(len(column.classes))
        ]
        writer.writerow(header)
        for time, z_top, z_bottom in requests:
            concs = column.concentration(time, z_top, z_bottom)
            writer.writerow(
                [f"{time:.6e}", f"{z_top:.6e}", f"{z_bottom:.6e}"]
                + [f"{c:.6e}" for c in concs]
            )


def write_mass_balance(column, requests, path):
    times = sorted({req[0] for req in requests})
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        n = len(column.classes)
        header = ["time_s"]
        for i in range(n):
            header += [
                f"suspended_class_{i}_kg",
                f"deposited_class_{i}_kg",
                f"total_class_{i}_kg",
            ]
        header += ["suspended_total_kg", "deposited_total_kg", "total_kg"]
        writer.writerow(header)
        for time in times:
            suspended, deposited, totals = column.mass_balance(time)
            row = [f"{time:.6e}"]
            for s, d, tot in zip(suspended, deposited, totals):
                row += [f"{s:.6e}", f"{d:.6e}", f"{tot:.6e}"]
            row += [
                f"{sum(suspended):.6e}",
                f"{sum(deposited):.6e}",
                f"{sum(totals):.6e}",
            ]
            writer.writerow(row)


def build_parser():
    parser = argparse.ArgumentParser(
        prog="settling",
        description=(
            "Still-water settling column analysis: terminal velocities, "
            "sampling-layer concentrations and mass balance."
        ),
    )
    parser.add_argument("scenario", help="scenario JSON file")
    parser.add_argument("samples", help="sampling requests CSV (time,z_top,z_bottom)")
    parser.add_argument(
        "--out-prefix",
        default="settling",
        help="output file prefix (default: settling)",
    )
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        column = load_scenario(args.scenario)
        requests = read_samples(args.samples)
        write_velocities(column, args.out_prefix + "_velocities.csv")
        write_concentrations(column, requests, args.out_prefix + "_concentrations.csv")
        write_mass_balance(column, requests, args.out_prefix + "_mass_balance.csv")
    except (ValidationError, ConvergenceError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    for i, term in enumerate(column.terminal):
        print(
            f"class {i}: v={term.velocity:.6e} m/s, Re={term.reynolds:.4g}, "
            f"residual={term.relative_residual:.2e}"
        )
    return 0


if __name__ == "__main__":
    sys.exit(main())
