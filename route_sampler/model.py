"""Route data model and per-segment Vincenty computation."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .vincenty import InverseResult, VincentyNotConverged, inverse


class RouteInputError(Exception):
    """Invalid waypoint or parameter, located by route and row."""

    def __init__(self, route_id: str, row: int | None, message: str):
        self.route_id = route_id
        self.row = row
        location = f"route '{route_id}'"
        if row is not None:
            location += f" row {row}"
        super().__init__(f"{location}: {message}")


@dataclass(frozen=True)
class Ellipsoid:
    a: float
    f: float

    def __post_init__(self) -> None:
        if not math.isfinite(self.a) or self.a <= 0.0:
            raise ValueError(f"semi-major axis must be positive finite, got {self.a!r}")
        if not math.isfinite(self.f) or not (0.0 <= self.f < 0.01):
            raise ValueError(f"flattening must satisfy 0 <= f < 0.01, got {self.f!r}")


WGS84 = Ellipsoid(a=6378137.0, f=1.0 / 298.257223563)


@dataclass(frozen=True)
class Waypoint:
    lat: float
    lon: float
    row: int


@dataclass
class Segment:
    index: int
    start: Waypoint
    end: Waypoint
    distance: float
    azimuth: float | None
    cumulative_start: float = 0.0
    cumulative_end: float = 0.0


@dataclass
class SegmentFailure:
    segment_index: int
    start_row: int
    end_row: int
    reason: str


@dataclass
class RouteResult:
    route_id: str
    waypoints: list[Waypoint]
    segments: list[Segment] = field(default_factory=list)
    failures: list[SegmentFailure] = field(default_factory=list)

    @property
    def ok(self) -> bool:
        return not self.failures

    @property
    def total_distance(self) -> float | None:
        if not self.ok:
            return None
        return sum(s.distance for s in self.segments)


def validate_waypoint(route_id: str, row: int, lat: float, lon: float) -> Waypoint:
    if not math.isfinite(lat) or not math.isfinite(lon):
        raise RouteInputError(route_id, row, "latitude/longitude must be finite")
    if not -90.0 <= lat <= 90.0:
        raise RouteInputError(route_id, row, f"latitude {lat} out of [-90, 90]")
    if lat == 90.0 or lat == -90.0:
        raise RouteInputError(route_id, row, "pole waypoints are not supported")
    if not -360.0 <= lon <= 360.0:
        raise RouteInputError(route_id, row, f"longitude {lon} out of [-360, 360]")
    return Waypoint(lat=lat, lon=lon, row=row)


def compute_segments(
    route_id: str,
    waypoints: list[Waypoint],
    ellipsoid: Ellipsoid,
    tol: float,
    max_iter: int,
) -> RouteResult:
    """Compute Vincenty inverse per adjacent pair; failures mark the route."""
    result = RouteResult(route_id=route_id, waypoints=waypoints)
    cumulative = 0.0
    for index in range(len(waypoints) - 1):
        start, end = waypoints[index], waypoints[index + 1]
        try:
            inv: InverseResult = inverse(
                start.lat, start.lon, end.lat, end.lon,
                ellipsoid.a, ellipsoid.f, tol, max_iter,
            )
        except VincentyNotConverged as exc:
            result.failures.append(
                SegmentFailure(
                    segment_index=index,
                    start_row=start.row,
                    end_row=end.row,
                    reason=str(exc),
                )
            )
            continue
        azimuth = inv.azimuth1 if inv.distance > 0.0 else None
        segment = Segment(
            index=index,
            start=start,
            end=end,
            distance=inv.distance,
            azimuth=azimuth,
            cumulative_start=cumulative,
            cumulative_end=cumulative + inv.distance,
        )
        cumulative += inv.distance
        result.segments.append(segment)
    return result
