"""Core gapped overlapping Allan deviation computations.

Times are seconds, fractional frequency values are dimensionless.
Only the Python standard library is used.
"""

from __future__ import annotations

import math
from dataclasses import dataclass


class AnalysisError(Exception):
    """Raised for invalid input data or configuration."""


@dataclass
class Segment:
    """One contiguous run of samples on the uniform grid."""

    start_index: int  # grid index of first sample
    times: list[float]
    values: list[float]

    @property
    def size(self) -> int:
        return len(self.values)


@dataclass
class TauResult:
    """Result for one averaging length m (tau = m * tau0)."""

    m: int
    tau: float
    adev: float | None  # None when not computable
    avar: float | None
    pairs: int  # number of valid squared differences
    status: str  # "ok" or "insufficient_data"


def build_segments(
    times: list[float],
    values: list[float],
    tau0: float,
    *,
    grid_tol: float = 1e-9,
) -> list[Segment]:
    """Validate the uniform grid and split data at gaps.

    Times must be strictly increasing and every sample must lie on the
    grid t_k = t_0 + k * tau0.  Missing grid points split the data into
    independent contiguous segments; nothing is interpolated.
    """
    if tau0 <= 0 or not math.isfinite(tau0):
        raise AnalysisError("base sample interval tau0 must be positive and finite")
    if len(times) != len(values):
        raise AnalysisError("time and value column lengths differ")
    if len(times) < 2:
        raise AnalysisError("at least two samples are required")

    t0 = times[0]

    def grid_index(t: float) -> int:
        k = (t - t0) / tau0
        r = round(k)
        if abs(k - r) > grid_tol:
            raise AnalysisError(
                f"time {t!r} does not lie on the uniform grid (tau0={tau0!r})"
            )
        return int(r)

    segments: list[Segment] = []
    cur_times = [times[0]]
    cur_values = [values[0]]
    cur_start = 0
    prev_index = 0

    for i in range(1, len(times)):
        t = times[i]
        if not (t > times[i - 1]):
            raise AnalysisError(
                f"timestamps must be strictly increasing (row {i + 1})"
            )
        k = grid_index(t)
        if k == prev_index + 1:
            cur_times.append(t)
            cur_values.append(values[i])
        else:
            segments.append(Segment(cur_start, cur_times, cur_values))
            cur_start = k
            cur_times = [t]
            cur_values = [values[i]]
        prev_index = k
    segments.append(Segment(cur_start, cur_times, cur_values))
    return segments
    segments.append(Segment(cur_start, cur_times, cur_values))
    return segments


def overlapping_allan(
    segments: list[Segment],
    m_values: list[int],
    tau0: float,
) -> list[TauResult]:
    """Overlapping Allan deviation across independent segments.

    For each averaging length m, every segment contributes sliding
    adjacent window pairs (each of m samples).  Results are pooled by
    summing squared differences and pair counts across segments; the
    variance is sum(d^2) / (2 * N) where N is the total pair count.
    """
    results: list[TauResult] = []
    for m in m_values:
        if not isinstance(m, int) or m < 1:
            raise AnalysisError(
                f"averaging length must be a positive integer, got {m!r}"
            )
        sum_sq = 0.0
        pairs = 0
        for seg in segments:
            n = seg.size
            if n < 2 * m:
                continue
            y = seg.values
            win = sum(y[0:m])
            next_win = sum(y[m : 2 * m])
            diff = (next_win - win) / m
            sum_sq += diff * diff
            pairs += 1
            for i in range(1, n - 2 * m + 1):
                win += y[i + m - 1] - y[i - 1]
                next_win += y[i + 2 * m - 1] - y[i + m - 1]
                diff = (next_win - win) / m
                sum_sq += diff * diff
                pairs += 1
        tau = m * tau0
        if pairs == 0:
            results.append(
                TauResult(m=m, tau=tau, adev=None, avar=None, pairs=0,
                          status="insufficient_data")
            )
        else:
            avar = sum_sq / (2.0 * pairs)
            results.append(
                TauResult(m=m, tau=tau, adev=math.sqrt(avar), avar=avar,
                          pairs=pairs, status="ok")
            )
    return results


def detrend_ols(
    times: list[float], values: list[float]
) -> tuple[float, float, list[float]]:
    """Ordinary least squares linear fit over actual observation times.

    Returns (slope, intercept, residuals).  Gaps are not compressed:
    the fit uses the real timestamps.  Residuals keep the original
    sample order so segment structure is preserved.
    """
    n = len(times)
    if n < 2:
        raise AnalysisError("at least two samples are required for a trend fit")
    st = sum(times)
    sy = sum(values)
    stt = sum(t * t for t in times)
    sty = sum(t * y for t, y in zip(times, values))
    denom = n * stt - st * st
    if denom == 0.0:
        raise AnalysisError("cannot fit trend: degenerate timestamps")
    slope = (n * sty - st * sy) / denom
    intercept = (sy - slope * st) / n
    residuals = [y - (slope * t + intercept) for t, y in zip(times, values)]
    return slope, intercept, residuals
