"""Command line interface: velocities, sampling concentrations, mass report."""
import argparse
import csv
import math
import sys

from .config import ConfigError, load_config
from .transport import interval_concentration, mass_balance
from .velocity import ConvergenceError, solve_terminal_velocity


def _read_samples(path):
    samples = []
    with open(path, "r", encoding="utf-8", newline="") as handle:
        for row in csv.DictReader(handle):
            try:
                time = float(row["time"])
                z_top = float(row["z_top"])
                z_bottom = float(row["z_bottom"])
            except (KeyError, TypeError, ValueError) as exc:
                raise ConfigError(
                    f"invalid sample row {row!r}: need time, z_top, z_bottom"
                ) from exc
            for value in (time, z_top, z_bottom):
                if not math.isfinite(value):
                    raise ConfigError(f"sample values must be finite: {row!r}")
            samples.append((time, z_top, z_bottom))
    if not samples:
        raise ConfigError("sample CSV contains no rows")
    return samples


def _write_velocities(path, classes, results):
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            ["class", "diameter_m", "density_kg_m3", "velocity_m_s",
             "reynolds", "drag_coefficient", "relative_residual", "iterations"]
        )
        for cls, res in zip(classes, results):
            writer.writerow(
                [cls.name, cls.diameter, cls.density, res.velocity,
                 res.reynolds, res.drag_coefficient, res.relative_residual,
                 res.iterations]
            )


def _write_concentrations(path, config, velocities, samples):
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            ["time_s", "z_top_m", "z_bottom_m", "class", "concentration_kg_m3"]
        )
        for time, z_top, z_bottom in samples:
            for cls, velocity in zip(config.classes, velocities):
                conc = interval_concentration(
                    cls.mass, velocity, time, config.column, z_top, z_bottom
                )
                writer.writerow([time, z_top, z_bottom, cls.name, conc])


def _write_mass_report(path, config, velocities, times):
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            ["time_s", "class", "initial_kg", "suspended_kg",
             "deposited_kg", "closure_error_kg"]
        )
        for time in times:
            rows, totals = mass_balance(
                config.classes, velocities, time, config.column.depth
            )
            for row in (*rows, totals):
                writer.writerow(
                    [time, row["name"], row["initial"], row["suspended"],
                     row["deposited"], row["closure_error"]]
                )


def build_parser():
    parser = argparse.ArgumentParser(
        prog="settling",
        description="Hydrostatic settling column analysis (SI units).",
    )
    parser.add_argument("config", help="JSON configuration file")
    parser.add_argument(
        "--samples",
        help="CSV with columns time,z_top,z_bottom (s, m, m); optional",
    )
    parser.add_argument(
        "--times",
        nargs="*",
        type=float,
        default=None,
        help="extra times (s) for the mass balance report",
    )
    parser.add_argument(
        "--outdir",
        default=".",
        help="output directory for CSV reports (default: current directory)",
    )
    return parser


def main(argv=None):
    args = build_parser().parse_args(argv)
    try:
        config = load_config(args.config)
        results = [
            solve_terminal_velocity(
                cls.diameter, cls.density, config.fluid, config.gravity
            )
            for cls in config.classes
        ]
    except (ConfigError, ConvergenceError, OSError, ValueError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    velocities = [res.velocity for res in results]
    for cls, res in zip(config.classes, results):
        print(
            f"{cls.name}: v={res.velocity:.6e} m/s, Re={res.reynolds:.4g}, "
            f"Cd={res.drag_coefficient:.4f}, residual={res.relative_residual:.2e}"
        )

    import os

    os.makedirs(args.outdir, exist_ok=True)
    _write_velocities(os.path.join(args.outdir, "velocities.csv"),
                      config.classes, results)

    times = set(args.times or [])
    if args.samples:
        try:
            samples = _read_samples(args.samples)
            _write_concentrations(
                os.path.join(args.outdir, "concentrations.csv"),
                config, velocities, samples,
            )
        except (ConfigError, ValueError, OSError) as exc:
            print(f"error: {exc}", file=sys.stderr)
            return 1
        times.update(s[0] for s in samples)
    if not times:
        times.add(0.0)
    _write_mass_report(
        os.path.join(args.outdir, "mass_balance.csv"),
        config, velocities, sorted(times),
    )
    print(f"reports written to {args.outdir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
