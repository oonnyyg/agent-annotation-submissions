"""Route model: segmented distances and equal-spacing sampling."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .vincenty import (
    Ellipsoid,
    VincentyError,
    direct,
    inverse,
)


@dataclass(frozen=True)
class Waypoint:
    lat: float
    lon: float
    row: int  # 1-based CSV data row for diagnostics


@dataclass
class Segment:
    index: int  # 0-based segment index
    start: Waypoint
    end: Waypoint
    distance: float
    initial_bearing: float  # nan when zero-length
    final_bearing: float
    cumulative_start: float = 0.0
    cumulative_end: float = 0.0


@dataclass
class RouteResult:
    route_id: str
    segments: list[Segment] = field(default_factory=list)
    total_distance: float = 0.0
    failed_segment: int | None = None
    error: str | None = None

    @property
    def ok(self) -> bool:
        return self.error is None


def compute_segments(route_id: str, waypoints: list[Waypoint],
                     ell: Ellipsoid, tol: float,
                     max_iter: int) -> RouteResult:
    """Compute per-segment inverse solutions and cumulative distances."""
    result = RouteResult(route_id=route_id)
    cumulative = 0.0
    for index in range(len(waypoints) - 1):
        start, end = waypoints[index], waypoints[index + 1]
        try:
            inv = inverse(ell, start.lat, start.lon, end.lat, end.lon,
                          tol=tol, max_iter=max_iter)
        except VincentyError as exc:
            result.error = (
                f"segment {index} (rows {start.row}-{end.row}) failed: {exc}"
            )
            result.failed_segment = index
            result.segments = []
            result.total_distance = math.nan
            return result
        segment = Segment(
            index=index,
            start=start,
            end=end,
            distance=inv.distance,
            initial_bearing=inv.initial_bearing,
            final_bearing=inv.final_bearing,
            cumulative_start=cumulative,
            cumulative_end=cumulative + inv.distance,
        )
        cumulative += inv.distance
        result.segments.append(segment)
    result.total_distance = cumulative
    return result


@dataclass(frozen=True)
class Sample:
    sequence: int
    lat: float
    lon: float
    cumulative: float  # metres along route
    segment_index: int  # owning segment; boundary samples fixed by rule


def sample_route(route: RouteResult, waypoints: list[Waypoint],
                 ell: Ellipsoid, step: float, tol: float,
                 max_iter: int) -> list[Sample]:
    """Sample a route at equal cumulative-distance intervals.

    The step remainder carries across segments. The final waypoint is
    always appended unless it duplicates the last sample. A route whose
    waypoints all coincide yields exactly one sample.

    Boundary rule: a sample landing exactly on a shared waypoint belongs
    to the segment that starts there (the later segment).
    """
    if not route.ok:
        raise ValueError("cannot sample a failed route")
    if not waypoints:
        return []
    if not route.segments:
        # single waypoint
        wp = waypoints[0]
        return [Sample(0, wp.lat, wp.lon, 0.0, 0)]

    samples: list[Sample] = []
    first = waypoints[0]
    samples.append(Sample(0, first.lat, first.lon, 0.0, 0))

    total = route.total_distance
    if total > 0.0:
        target = step
        seg_pos = 0  # index of segment that may contain the target
        while target < total:
            while (seg_pos < len(route.segments) - 1
                   and target >= route.segments[seg_pos].cumulative_end):
                seg_pos += 1
            segment = route.segments[seg_pos]
            if segment.distance > 0.0:
                along = target - segment.cumulative_start
                out = direct(ell, segment.start.lat, segment.start.lon,
                             segment.initial_bearing, along,
                             tol=tol, max_iter=max_iter)
                lat, lon = out.lat, out.lon
            else:
                lat, lon = segment.start.lat, segment.start.lon
            samples.append(Sample(len(samples), lat, lon, target, seg_pos))
            target += step

    last = waypoints[-1]
    last_sample = samples[-1]
    duplicate = (
        last_sample.lat == last.lat
        and last_sample.lon == last.lon
        and last_sample.cumulative == total
    )
    if not duplicate:
        samples.append(
            Sample(len(samples), last.lat, last.lon, total,
                   len(route.segments) - 1)
        )
    return samples
