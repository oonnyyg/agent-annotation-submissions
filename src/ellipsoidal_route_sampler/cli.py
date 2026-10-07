"""Command line interface: reads routes/settings, writes CSV/JSON outputs."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys
from pathlib import Path

from .io_utils import InputError, load_routes, load_settings
from .route import compute_segments, sample_route


def _fmt(value: float) -> str:
    if isinstance(value, float) and math.isnan(value):
        return ""
    return f"{value:.10f}" if isinstance(value, float) else str(value)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="route-sampler",
        description="Ellipsoidal route segment distances and "
                    "equal-spacing sampler (Vincenty, offline).")
    parser.add_argument("--routes", type=Path, required=True,
                        help="CSV with columns route_id,lat,lon")
    parser.add_argument("--settings", type=Path, required=True,
                        help="JSON with ellipsoid and sampling settings")
    parser.add_argument("--out-dir", type=Path, required=True,
                        help="directory for segments.csv, samples.csv, "
                             "diagnostics.json")
    return parser


def run(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        settings = load_settings(args.settings)
        routes = load_routes(args.routes)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    args.out_dir.mkdir(parents=True, exist_ok=True)
    segment_rows: list[dict[str, str]] = []
    sample_rows: list[dict[str, str]] = []
    diagnostics: dict[str, object] = {
        "ellipsoid": {
            "semi_major_axis": settings.ellipsoid.a,
            "flattening": settings.ellipsoid.f,
        },
        "sample_interval": settings.step,
        "tolerance": settings.tol,
        "max_iterations": settings.max_iter,
        "routes": {},
    }
    route_diags: dict[str, object] = diagnostics["routes"]  # type: ignore[assignment]

    for route_id, waypoints in routes.items():
        result = compute_segments(route_id, waypoints, settings.ellipsoid,
                                  settings.tol, settings.max_iter)
        if not result.ok:
            route_diags[route_id] = {
                "status": "failed",
                "failed_segment": result.failed_segment,
                "error": result.error,
            }
            continue
        for segment in result.segments:
            segment_rows.append({
                "route_id": route_id,
                "segment_index": str(segment.index),
                "start_lat": _fmt(segment.start.lat),
                "start_lon": _fmt(segment.start.lon),
                "end_lat": _fmt(segment.end.lat),
                "end_lon": _fmt(segment.end.lon),
                "distance_m": _fmt(segment.distance),
                "initial_bearing_deg": _fmt(segment.initial_bearing),
                "final_bearing_deg": _fmt(segment.final_bearing),
                "cumulative_start_m": _fmt(segment.cumulative_start),
                "cumulative_end_m": _fmt(segment.cumulative_end),
            })
        samples = sample_route(result, waypoints, settings.ellipsoid,
                               settings.step, settings.tol,
                               settings.max_iter)
        for sample in samples:
            sample_rows.append({
                "route_id": route_id,
                "sequence": str(sample.sequence),
                "lat": _fmt(sample.lat),
                "lon": _fmt(sample.lon),
                "cumulative_m": _fmt(sample.cumulative),
                "segment_index": str(sample.segment_index),
            })
        route_diags[route_id] = {
            "status": "ok",
            "waypoint_count": len(waypoints),
            "segment_count": len(result.segments),
            "total_distance_m": result.total_distance,
            "sample_count": len(samples),
        }

    _write_csv(args.out_dir / "segments.csv", segment_rows,
               ["route_id", "segment_index", "start_lat", "start_lon",
                "end_lat", "end_lon", "distance_m", "initial_bearing_deg",
                "final_bearing_deg", "cumulative_start_m",
                "cumulative_end_m"])
    _write_csv(args.out_dir / "samples.csv", sample_rows,
               ["route_id", "sequence", "lat", "lon", "cumulative_m",
                "segment_index"])
    diag_path = args.out_dir / "diagnostics.json"
    diag_path.write_text(
        json.dumps(diagnostics, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8")
    failed = sum(1 for d in route_diags.values()
                 if d["status"] != "ok")  # type: ignore[index]
    print(f"processed {len(routes)} route(s), {failed} failed; "
          f"outputs in {args.out_dir}")
    return 1 if failed else 0


def _write_csv(path: Path, rows: list[dict[str, str]],
               fieldnames: list[str]) -> None:
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)


if __name__ == "__main__":
    sys.exit(run())
