"""Command line entry: python -m route_sampler WAYPOINTS_CSV SETTINGS_JSON."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .io import (
    load_routes,
    load_settings,
    write_diagnostics,
    write_samples,
    write_segments,
)
from .model import compute_segments
from .sampler import sample_route


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="route_sampler",
        description=(
            "Compute ellipsoidal (Vincenty) segment distances and "
            "equal-distance samples for ordered waypoint routes."
        ),
    )
    parser.add_argument("waypoints", help="CSV with columns route_id,lat,lon")
    parser.add_argument("settings", help="JSON with ellipsoid and sampling settings")
    parser.add_argument(
        "--out-dir", default=".", help="output directory (default: current)"
    )
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        settings = load_settings(args.settings)
        routes = load_routes(args.waypoints)
    except (ValueError, OSError, KeyError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    results = []
    samples_by_route = {}
    for route_id, waypoints in routes.items():
        result = compute_segments(
            route_id, waypoints,
            settings.ellipsoid, settings.tol, settings.max_iter,
        )
        results.append(result)
        if result.ok:
            samples_by_route[route_id] = sample_route(
                result, settings.spacing,
                settings.ellipsoid, settings.tol, settings.max_iter,
            )
        else:
            for failure in result.failures:
                print(
                    f"warning: route '{route_id}' segment {failure.segment_index} "
                    f"(rows {failure.start_row}-{failure.end_row}): {failure.reason}",
                    file=sys.stderr,
                )

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    write_segments(out_dir / "segments.csv", results)
    write_samples(out_dir / "samples.csv", samples_by_route)
    write_diagnostics(out_dir / "diagnostics.json", results, samples_by_route, settings)

    failed = [r.route_id for r in results if not r.ok]
    if failed:
        print(f"failed routes (no totals/samples emitted): {failed}", file=sys.stderr)
        return 2
    return 0


if __name__ == "__main__":
    sys.exit(main())
