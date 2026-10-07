"""Core computations for gapped overlapping Allan deviation.

Times are seconds, values are dimensionless fractional frequency. The
nominal sampling interval ``tau0`` defines a uniform grid; missing grid
points split the record into independent contiguous segments. No
interpolation is performed and averaging windows never cross a gap.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

GRID_REL_TOL = 1e-9


class GridError(ValueError):
    """Raised when timestamps do not lie on the declared uniform grid."""


@dataclass
class Segment:
    """One contiguous run of samples on the grid."""

    start_index: int  # grid index of first sample
    times: list[float]
    values: list[float]

    @property
    def size(self) -> int:
        return len(self.values)


@dataclass
class ScaleResult:
    """Overlapping Allan statistics for one averaging length m."""

    m: int
    tau: float  # averaging time, m * tau0
    windows: int  # effective number of squared mean differences
    deviation: float | None  # None when not computable
    variance: float | None
    status: str  # "ok" or "insufficient_data"


@dataclass
class AnalysisResult:
    tau0: float
    detrended: bool
    scales: list[ScaleResult]
    segments: list[int]  # sizes of contiguous segments
    n_samples: int
    fit: dict | None = None  # {"slope": ..., "intercept": ...} per second
    raw_scales: list[ScaleResult] | None = None  # pre-detrend results


def grid_indices(times: list[float], tau0: float) -> list[int]:
    """Map timestamps to integer grid indices, validating the grid."""
    if tau0 <= 0 or not math.isfinite(tau0):
        raise GridError(f"tau0 must be a positive finite number, got {tau0!r}")
    if not times:
        raise GridError("no timestamps provided")
    tol = GRID_REL_TOL * tau0
    indices: list[int] = []
    prev_k: int | None = None
    t0 = times[0]
    for t in times:
        if not math.isfinite(t):
            raise GridError(f"non-finite timestamp {t!r}")
        k = round((t - t0) / tau0)
        if abs(t - (t0 + k * tau0)) > tol:
            raise GridError(
                f"timestamp {t!r} does not lie on the grid "
                f"(tau0={tau0!r}, origin={t0!r})"
            )
        if prev_k is not None and k <= prev_k:
            raise GridError("timestamps must be strictly increasing")
        indices.append(k)
        prev_k = k
    return indices


def split_segments(
    times: list[float], values: list[float], tau0: float
) -> list[Segment]:
    """Split observations into contiguous segments at sampling gaps."""
    if len(times) != len(values):
        raise GridError("times and values must have equal length")
    indices = grid_indices(times, tau0)
    segments: list[Segment] = []
    cur_times = [times[0]]
    cur_values = [values[0]]
    cur_start = indices[0]
    for i in range(1, len(times)):
        if indices[i] == indices[i - 1] + 1:
            cur_times.append(times[i])
            cur_values.append(values[i])
        else:
            segments.append(Segment(cur_start, cur_times, cur_values))
            cur_times = [times[i]]
            cur_values = [values[i]]
            cur_start = indices[i]
    segments.append(Segment(cur_start, cur_times, cur_values))
    return segments


def _segment_sum_sq_diff(values: list[float], m: int) -> tuple[float, int]:
    """Sum of squared differences of adjacent length-m means (overlapping)."""
    n = len(values)
    if n < 2 * m:
        return 0.0, 0
    # Prefix sums for O(1) window means.
    prefix = [0.0] * (n + 1)
    for i, v in enumerate(values):
        prefix[i + 1] = prefix[i] + v
    total = 0.0
    count = 0
    for i in range(0, n - 2 * m + 1):
        mean_a = (prefix[i + m] - prefix[i]) / m
        mean_b = (prefix[i + 2 * m] - prefix[i + m]) / m
        diff = mean_b - mean_a
        total += diff * diff
        count += 1
    return total, count


def overlapping_allan(
    segments: list[Segment], m: int, tau0: float
) -> ScaleResult:
    """Overlapping Allan deviation pooled across segments by window count."""
    if m < 1:
        raise ValueError(f"averaging length m must be >= 1, got {m!r}")
    total = 0.0
    count = 0
    for seg in segments:
        s, c = _segment_sum_sq_diff(seg.values, m)
        total += s
        count += c
    tau = m * tau0
    if count == 0:
        return ScaleResult(m, tau, 0, None, None, "insufficient_data")
    variance = total / (2.0 * count)
    return ScaleResult(m, tau, count, math.sqrt(variance), variance, "ok")


def ols_fit(times: list[float], values: list[float]) -> tuple[float, float]:
    """Ordinary least squares fit value = slope * t + intercept."""
    n = len(times)
    mean_t = sum(times) / n
    mean_v = sum(values) / n
    s_tt = sum((t - mean_t) ** 2 for t in times)
    if s_tt == 0.0:
        return 0.0, mean_v
    s_tv = sum((t - mean_t) * (v - mean_v) for t, v in zip(times, values))
    slope = s_tv / s_tt
    return slope, mean_v - slope * mean_t


def _scales_for(
    segments: list[Segment], m_list: list[int], tau0: float
) -> list[ScaleResult]:
    return [overlapping_allan(segments, m, tau0) for m in m_list]


def analyze(
    times: list[float],
    values: list[float],
    tau0: float,
    m_list: list[int],
    detrend: bool = False,
) -> AnalysisResult:
    """Run the full analysis; optionally remove an OLS linear trend first.

    Detrending uses the actual timestamps, so the time distance across a
    gap is never compressed. Results are sorted by averaging time.
    """
    if not m_list:
        raise ValueError("at least one averaging length m is required")
    m_sorted = sorted(set(m_list))
    segments = split_segments(times, values, tau0)
    if detrend:
        slope, intercept = ols_fit(times, values)
        raw_segments = segments
        detrended_segments = [
            Segment(
                seg.start_index,
                seg.times,
                [v - (slope * t + intercept) for t, v in zip(seg.times, seg.values)],
            )
            for seg in segments
        ]
        return AnalysisResult(
            tau0=tau0,
            detrended=True,
            scales=_scales_for(detrended_segments, m_sorted, tau0),
            segments=[seg.size for seg in segments],
            n_samples=len(times),
            fit={"slope": slope, "intercept": intercept},
            raw_scales=_scales_for(raw_segments, m_sorted, tau0),
        )
    return AnalysisResult(
        tau0=tau0,
        detrended=False,
        scales=_scales_for(segments, m_sorted, tau0),
        segments=[seg.size for seg in segments],
        n_samples=len(times),
    )
