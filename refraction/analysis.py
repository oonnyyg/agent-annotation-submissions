"""First-arrival selection and branch-dominance analysis."""

from __future__ import annotations

from dataclasses import dataclass

from .paths import Path

DEFAULT_TIME_TOL = 1e-9  # seconds; ties within this are all kept


@dataclass
class FirstArrival:
    offset: float
    time: float
    paths: list[Path]  # all paths tied within tolerance


def first_arrivals(
    offsets: list[float],
    paths: list[Path],
    time_tol: float = DEFAULT_TIME_TOL,
) -> list[FirstArrival]:
    """Pick the earliest reachable path(s) at each offset."""
    results = []
    for offset in offsets:
        if offset < 0.0:
            raise ValueError(f"offset must be non-negative, got {offset}")
        best_time = None
        best_paths: list[Path] = []
        for path in paths:
            if not path.reachable(offset):
                continue
            t = path.travel_time(offset)
            if best_time is None or t < best_time - time_tol:
                best_time = t
                best_paths = [path]
            elif t <= best_time + time_tol:
                best_paths.append(path)
        results.append(
            FirstArrival(offset=offset, time=best_time, paths=best_paths)
        )
    return results


def _time_difference(path_a: Path, path_b: Path, offset: float) -> float:
    return path_a.travel_time(offset) - path_b.travel_time(offset)


def _find_crossover(
    path_a: Path, path_b: Path, lo: float, hi: float, x_tol: float = 1e-10
) -> float | None:
    """Bisect for the offset where two linear branches intersect.

    Both branches are linear in offset, so at most one crossing exists.
    """
    f_lo = _time_difference(path_a, path_b, lo)
    f_hi = _time_difference(path_a, path_b, hi)
    if abs(f_lo) < 1e-15:
        return lo
    if f_lo * f_hi > 0.0:
        return None
    while hi - lo > x_tol:
        mid = 0.5 * (lo + hi)
        f_mid = _time_difference(path_a, path_b, mid)
        if f_lo * f_mid <= 0.0:
            hi = mid
            f_hi = f_mid
        else:
            lo = mid
            f_lo = f_mid
    return 0.5 * (lo + hi)


def dominance_intervals(
    paths: list[Path],
    offset_min: float,
    offset_max: float,
    time_tol: float = DEFAULT_TIME_TOL,
) -> dict:
    """Compute where each path is the first arrival within [min, max].

    Branch switch points are true intersections of the linear travel-time
    curves, restricted to offsets where both paths are reachable.
    """
    if offset_min < 0.0 or offset_max <= offset_min:
        raise ValueError("require 0 <= offset_min < offset_max")

    # Candidate boundaries: range ends, critical distances, crossovers.
    boundaries = {offset_min, offset_max}
    for path in paths:
        if offset_min < path.critical_distance < offset_max:
            boundaries.add(path.critical_distance)
    for i, pa in enumerate(paths):
        for pb in paths[i + 1:]:
            lo = max(offset_min, pa.critical_distance, pb.critical_distance)
            if lo >= offset_max:
                continue
            cross = _find_crossover(pa, pb, lo, offset_max)
            if cross is not None and offset_min < cross < offset_max:
                boundaries.add(cross)
    ordered = sorted(boundaries)

    # Evaluate the winner at the midpoint of every elementary interval.
    intervals = []
    for lo, hi in zip(ordered, ordered[1:]):
        mid = 0.5 * (lo + hi)
        winner = first_arrivals([mid], paths, time_tol)[0]
        intervals.append(
            {
                "offset_min": lo,
                "offset_max": hi,
                "winners": [
                    p.refractor if p.kind == "refracted" else "direct"
                    for p in winner.paths
                ],
            }
        )

    # Merge adjacent intervals with identical winners.
    merged: list[dict] = []
    for iv in intervals:
        if merged and merged[-1]["winners"] == iv["winners"]:
            merged[-1]["offset_max"] = iv["offset_max"]
        else:
            merged.append(dict(iv))

    dominant = {w for iv in merged for w in iv["winners"]}
    never_first = [
        (p.refractor if p.kind == "refracted" else "direct")
        for p in paths
        if (p.refractor if p.kind == "refracted" else "direct") not in dominant
    ]
    # Crossovers are interior boundaries where the winning branch changes.
    crossovers = [
        iv["offset_max"]
        for iv, nxt in zip(merged, merged[1:])
        if iv["winners"] != nxt["winners"]
    ]
    return {
        "offset_min": offset_min,
        "offset_max": offset_max,
        "crossovers": crossovers,
        "intervals": merged,
        "never_first": never_first,
    }
