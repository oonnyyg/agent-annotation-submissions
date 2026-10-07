"""Command line interface for parallel-beam reconstruction."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys

from .config import ConfigError, load_config
from .disk import write_sinogram
from .fbp import reconstruct
from .projections import ProjectionError, load_projections


def _cmd_reconstruct(args: argparse.Namespace) -> int:
    geometry = load_config(args.config)
    projections = load_projections(args.projections, geometry.detector_count)

    image = reconstruct(projections, geometry)

    with open(args.output, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        for row in image:
            writer.writerow([f"{v:.10g}" for v in row])

    x_min, x_max = geometry.x_range()
    y_min, y_max = geometry.y_range()
    flat = [v for row in image for v in row]
    summary = {
        "image": {
            "nx": geometry.nx,
            "ny": geometry.ny,
            "dx": geometry.dx,
            "dy": geometry.dy,
            "x_range": [x_min, x_max],
            "y_range": [y_min, y_max],
        },
        "detector": {
            "count": geometry.detector_count,
            "spacing": geometry.detector_spacing,
        },
        "angles": {
            "count": len(projections),
            "min_deg": math.degrees(min(a for a, _ in projections)),
            "max_deg": math.degrees(max(a for a, _ in projections)),
            "step_deg": 180.0 / len(projections),
        },
        "filter": {
            "type": "ramp",
            "multiplier": "abs(f)",
            "frequency_unit": "cycles per unit length",
            "zero_padding": "next power of two >= 2 * detector_count",
            "normalization": "none",
        },
        "result": {
            "min": min(flat),
            "max": max(flat),
            "mean": sum(flat) / len(flat),
        },
    }
    with open(args.summary, "w", encoding="utf-8") as fh:
        json.dump(summary, fh, indent=2)
        fh.write("\n")
    return 0


def _cmd_make_disk(args: argparse.Namespace) -> int:
    geometry = load_config(args.config)
    step = 180.0 / args.angles
    angles = [i * step for i in range(args.angles)]
    write_sinogram(
        args.output,
        angles,
        geometry.detector_positions(),
        args.radius,
        args.attenuation,
        (args.center_x, args.center_y),
    )
    return 0


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="pbr",
        description="Parallel-beam filtered backprojection (synthetic data only)",
    )
    sub = parser.add_subparsers(dest="command", required=True)

    rec = sub.add_parser("reconstruct", help="reconstruct from projection CSV")
    rec.add_argument("--projections", required=True, help="projection CSV path")
    rec.add_argument("--config", required=True, help="geometry JSON path")
    rec.add_argument("--output", required=True, help="output matrix CSV path")
    rec.add_argument("--summary", required=True, help="output summary JSON path")
    rec.set_defaults(func=_cmd_reconstruct)

    disk = sub.add_parser("make-disk", help="write an analytic disk sinogram CSV")
    disk.add_argument("--config", required=True, help="geometry JSON path")
    disk.add_argument("--output", required=True, help="output sinogram CSV path")
    disk.add_argument("--angles", type=int, required=True, help="number of angles")
    disk.add_argument("--radius", type=float, required=True, help="disk radius")
    disk.add_argument(
        "--attenuation", type=float, required=True, help="disk attenuation"
    )
    disk.add_argument("--center-x", type=float, default=0.0)
    disk.add_argument("--center-y", type=float, default=0.0)
    disk.set_defaults(func=_cmd_make_disk)
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        return args.func(args)
    except (ConfigError, ProjectionError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2


if __name__ == "__main__":
    sys.exit(main())
