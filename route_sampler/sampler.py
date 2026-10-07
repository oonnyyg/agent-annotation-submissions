"""Equal-distance sampling along a route's cumulative mileage."""

from __future__ import annotations

from dataclasses import dataclass

from .model import Ellipsoid, RouteResult, Segment
from .vincenty import direct


@dataclass(frozen=True)
class Sample:
    index: int
    lat: float
    lon: float
    cumulative: float
    segment_index: int | None


def _attribute(segments: list[Segment], cumulative: float) -> int:
    """Fixed boundary rule for segment attribution.

    A sample at cumulative distance d belongs to the first positive-length
    segment whose cumulative_end >= d. A sample exactly on a shared boundary
    therefore belongs to the earlier segment; the final endpoint belongs to
    the last positive-length segment.
    """
    candidates = [s for s in segments if s.distance > 0.0]
    for seg in candidates:
        if cumulative <= seg.cumulative_end:
            return seg.index
    return candidates[-1].index


def sample_route(
    route: RouteResult,
    spacing: float,
    ellipsoid: Ellipsoid,
    tol: float,
    max_iter: int,
) -> list[Sample]:
    """Sample every `spacing` meters of cumulative distance.

    The step remainder carries across segments because targets are absolute
    cumulative distances. The final waypoint is appended unless a sample
    already coincides with it. A route whose waypoints all coincide yields
    exactly one sample at that position.
    """
    total = route.total_distance
    assert total is not None, "failed routes must not be sampled"

    if total == 0.0:
        first = route.waypoints[0]
        return [Sample(index=0, lat=first.lat, lon=first.lon, cumulative=0.0,
                       segment_index=None)]

    targets: list[float] = []
    k = 0
    while k * spacing < total:
        targets.append(k * spacing)
        k += 1
    targets.append(total)

    seg_by_index = {s.index: s for s in route.segments}
    samples: list[Sample] = []
    for index, target in enumerate(targets):
        seg_index = _attribute(route.segments, target)
        seg = seg_by_index[seg_index]
        offset = min(max(target - seg.cumulative_start, 0.0), seg.distance)
        if offset == 0.0:
            lat, lon = seg.start.lat, seg.start.lon
        elif offset == seg.distance:
            lat, lon = seg.end.lat, seg.end.lon
        else:
            assert seg.azimuth is not None
            point = direct(
                seg.start.lat, seg.start.lon, seg.azimuth, offset,
                ellipsoid.a, ellipsoid.f, tol, max_iter,
            )
            lat, lon = point.lat2, point.lon2
        samples.append(Sample(index=index, lat=lat, lon=lon,
                              cumulative=target, segment_index=seg_index))
    return samples
