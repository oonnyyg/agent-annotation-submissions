"""First-arrival branch intervals over a continuous offset range.

All candidate paths are linear in x, so the first-arrival curve is the
lower envelope of lines, each valid only above its critical distance.
Switch points are computed analytically from line intersections and
critical distances, never guessed from sampled receivers.
"""
from __future__ import annotations

from dataclasses import dataclass

from .model import VelocityModel
from .traveltime import Path, TIME_TOLERANCE, all_paths


@dataclass(frozen=True)
class BranchInterval:
    """Offset interval where a fixed set of paths ties for first arrival."""

    start: float
    end: float
    paths: tuple[Path, ...]


def _intersection(a: Path, b: Path) -> float | None:
    scale = max(1.0, abs(a.slope), abs(b.slope))
    if abs(a.slope - b.slope) <= 1e-15 * scale:
        return None
    return (b.intercept - a.intercept) / (a.slope - b.slope)


def _winners(paths: list[Path], x: float, tol: float) -> tuple[Path, ...]:
    valid = [p for p in paths if p.reachable(x, tol)]
    best = min(p.time(x) for p in valid)
    scale = max(1.0, abs(best))
    return tuple(p for p in valid if abs(p.time(x) - best) <= tol * scale)


def _key(path: Path) -> tuple[str, int | None]:
    return (path.kind, path.interface)


def branch_intervals(
    model: VelocityModel,
    x_min: float,
    x_max: float,
    tol: float = TIME_TOLERANCE,
) -> tuple[list[BranchInterval], list[Path]]:
    """Split [x_min, x_max] into first-arrival branches.

    Returns (intervals, never_first): intervals cover the range with the
    winning path(s) on each; never_first lists refracted paths that are
    reachable somewhere in the range but never form the first arrival.
    """
    if not 0.0 <= x_min < x_max:
        raise ValueError("require 0 <= x_min < x_max")
    paths = all_paths(model)

    points = {x_min, x_max}
    for path in paths:
        if x_min < path.min_offset < x_max:
            points.add(path.min_offset)
    for i, first in enumerate(paths):
        for second in paths[i + 1:]:
            x = _intersection(first, second)
            if x is None or not x_min < x < x_max:
                continue
            if first.reachable(x, tol) and second.reachable(x, tol):
                points.add(x)
    ordered = sorted(points)

    intervals: list[BranchInterval] = []
    for left, right in zip(ordered, ordered[1:]):
        winners = _winners(paths, 0.5 * (left + right), tol)
        if intervals and all(
            _key(p) == _key(q)
            for p, q in zip(intervals[-1].paths, winners)
        ) and len(intervals[-1].paths) == len(winners):
            prev = intervals[-1]
            intervals[-1] = BranchInterval(prev.start, right, prev.paths)
        else:
            intervals.append(BranchInterval(left, right, winners))

    winning = {_key(p) for iv in intervals for p in iv.paths}
    margin = tol * max(1.0, x_max)
    never_first = [
        p
        for p in paths
        if p.kind == "refracted"
        and p.min_offset < x_max - margin
        and _key(p) not in winning
    ]
    return intervals, never_first
