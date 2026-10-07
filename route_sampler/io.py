"""CSV/JSON input and output for the route sampler."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass
from pathlib import Path

from .model import Ellipsoid, RouteInputError, RouteResult, WGS84, Waypoint
from .model import validate_waypoint
from .sampler import Sample


@dataclass(frozen=True)
class Settings:
    ellipsoid: Ellipsoid
    spacing: float
    tol: float
    max_iter: int


def load_settings(path: str | Path) -> Settings:
    """Load ellipsoid and sampling settings from JSON; WGS84 by default."""
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    if not isinstance(raw, dict):
        raise ValueError("settings JSON must be an object")
    ell_raw = raw.get("ellipsoid", {})
    if not isinstance(ell_raw, dict):
        raise ValueError("'ellipsoid' must be an object")
    ellipsoid = Ellipsoid(
        a=float(ell_raw.get("a", WGS84.a)),
        f=float(ell_raw.get("f", WGS84.f)),
    )
    spacing = float(raw["sample_spacing_m"])
    if not math.isfinite(spacing) or spacing <= 0.0:
        raise ValueError(f"sample_spacing_m must be positive finite, got {spacing!r}")
    tol = float(raw.get("tolerance", 1e-12))
    if not math.isfinite(tol) or tol <= 0.0:
        raise ValueError(f"tolerance must be positive finite, got {tol!r}")
    max_iter = int(raw.get("max_iterations", 200))
    if max_iter <= 0:
        raise ValueError(f"max_iterations must be a positive integer, got {max_iter!r}")
    return Settings(ellipsoid=ellipsoid, spacing=spacing, tol=tol, max_iter=max_iter)


def load_routes(path: str | Path) -> dict[str, list[Waypoint]]:
    """Read an ordered waypoint CSV with columns route_id,lat,lon.

    Rows for the same route_id keep file order. Row numbers reported in
    errors are 1-based data rows (header excluded).
    """
    routes: dict[str, list[Waypoint]] = {}
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        required = {"route_id", "lat", "lon"}
        if reader.fieldnames is None or not required <= set(reader.fieldnames):
            raise ValueError(
                f"waypoint CSV must have columns {sorted(required)}, "
                f"got {reader.fieldnames!r}"
            )
        for row_no, record in enumerate(reader, start=1):
            route_id = (record["route_id"] or "").strip()
            if not route_id:
                raise RouteInputError("<missing>", row_no, "empty route_id")
            try:
                lat = float(record["lat"])
                lon = float(record["lon"])
            except (TypeError, ValueError):
                raise RouteInputError(
                    route_id, row_no,
                    f"unparseable lat/lon: {record['lat']!r}, {record['lon']!r}",
                ) from None
            waypoint = validate_waypoint(route_id, row_no, lat, lon)
            routes.setdefault(route_id, []).append(waypoint)
    return routes


SEGMENT_FIELDS = [
    "route_id", "segment_index", "start_lat", "start_lon", "end_lat", "end_lon",
    "distance_m", "azimuth_deg", "cumulative_start_m", "cumulative_end_m",
    "status",
]

SAMPLE_FIELDS = [
    "route_id", "sample_index", "lat", "lon", "cumulative_m", "segment_index",
]


def write_segments(path: str | Path, results: list[RouteResult]) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=SEGMENT_FIELDS)
        writer.writeheader()
        for result in results:
            status = "ok" if result.ok else "failed"
            for seg in result.segments:
                writer.writerow({
                    "route_id": result.route_id,
                    "segment_index": seg.index,
                    "start_lat": f"{seg.start.lat:.8f}",
                    "start_lon": f"{seg.start.lon:.8f}",
                    "end_lat": f"{seg.end.lat:.8f}",
                    "end_lon": f"{seg.end.lon:.8f}",
                    "distance_m": f"{seg.distance:.3f}",
                    "azimuth_deg": "" if seg.azimuth is None else f"{seg.azimuth:.5f}",
                    "cumulative_start_m": f"{seg.cumulative_start:.3f}",
                    "cumulative_end_m": f"{seg.cumulative_end:.3f}",
                    "status": status,
                })


def write_samples(
    path: str | Path,
    samples_by_route: dict[str, list[Sample]],
) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=SAMPLE_FIELDS)
        writer.writeheader()
        for route_id, samples in samples_by_route.items():
            for sample in samples:
                writer.writerow({
                    "route_id": route_id,
                    "sample_index": sample.index,
                    "lat": f"{sample.lat:.8f}",
                    "lon": f"{sample.lon:.8f}",
                    "cumulative_m": f"{sample.cumulative:.3f}",
                    "segment_index": (
                        "" if sample.segment_index is None else sample.segment_index
                    ),
                })


def write_diagnostics(
    path: str | Path,
    results: list[RouteResult],
    samples_by_route: dict[str, list[Sample]],
    settings: Settings,
) -> None:
    routes = []
    for result in results:
        entry = {
            "route_id": result.route_id,
            "status": "ok" if result.ok else "failed",
            "waypoint_count": len(result.waypoints),
            "segment_count": len(result.segments),
            "total_distance_m": result.total_distance,
            "sample_count": len(samples_by_route.get(result.route_id, [])),
            "failed_segments": [
                {
                    "segment_index": f.segment_index,
                    "start_row": f.start_row,
                    "end_row": f.end_row,
                    "reason": f.reason,
                }
                for f in result.failures
            ],
        }
        routes.append(entry)
    payload = {
        "ellipsoid": {"a": settings.ellipsoid.a, "f": settings.ellipsoid.f},
        "sample_spacing_m": settings.spacing,
        "tolerance": settings.tol,
        "max_iterations": settings.max_iter,
        "routes": routes,
    }
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=2, ensure_ascii=False)
        fh.write("\n")
