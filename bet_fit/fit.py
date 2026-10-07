"""BET linear transform, least-squares fit, interval screening and selection."""

from __future__ import annotations

import math
from dataclasses import dataclass

AVOGADRO = 6.02214076e23  # mol^-1, exact per SI


@dataclass(frozen=True)
class BetResult:
    start: int                      # index of first point (into sample arrays)
    end: int                        # index of last point, inclusive
    n_points: int
    p_start: float
    p_end: float
    slope: float
    intercept: float
    monolayer_mol_per_g: float      # n_m = 1 / (slope + intercept)
    bet_constant: float             # C = 1 + slope / intercept
    r_squared: float | None         # None when undefined (zero y variance)
    area_m2_per_g: float
    residuals: tuple[float, ...]    # y_fit - y_obs per point (BET space)
    rejections: tuple[str, ...] = ()

    @property
    def accepted(self) -> bool:
        return not self.rejections


def bet_transform(p: float, q: float) -> float:
    """BET ordinate: 1 / (q * (1/p - 1)) = p / (q * (1 - p))."""
    return p / (q * (1.0 - p))


def _linregress(xs, ys):
    n = len(xs)
    sx = sum(xs)
    sy = sum(ys)
    sxx = sum(x * x for x in xs)
    sxy = sum(x * y for x, y in zip(xs, ys))
    denom = n * sxx - sx * sx
    slope = (n * sxy - sx * sy) / denom
    intercept = (sy - slope * sx) / n
    ybar = sy / n
    ss_tot = sum((y - ybar) ** 2 for y in ys)
    ss_res = sum((y - (slope * x + intercept)) ** 2 for x, y in zip(xs, ys))
    # R^2 is undefined when the ordinate has no variance; allow for
    # floating-point noise around an exactly constant y.
    if ss_tot <= 1e-24 * max(ybar * ybar, 1e-300) * n:
        r2 = None
    else:
        r2 = 1.0 - ss_res / ss_tot
    residuals = tuple((slope * x + intercept) - y for x, y in zip(xs, ys))
    return slope, intercept, r2, residuals


def fit_interval(pressures, quantities, start, end, cross_section_nm2):
    """Fit points start..end (inclusive) and apply screening rules."""
    ps = list(pressures[start:end + 1])
    qs = list(quantities[start:end + 1])
    ys = [bet_transform(p, q) for p, q in zip(ps, qs)]
    slope, intercept, r2, residuals = _linregress(ps, ys)

    rejections = []
    if intercept <= 0.0:
        rejections.append("intercept not positive")
    n_m = math.nan
    c_const = math.nan
    area = math.nan
    if slope + intercept > 0.0:
        n_m = 1.0 / (slope + intercept)
    else:
        rejections.append("monolayer capacity not positive")
    if intercept > 0.0:
        c_const = 1.0 + slope / intercept
        if c_const <= 0.0:
            rejections.append("BET constant not positive")
    if math.isfinite(n_m):
        area = n_m * AVOGADRO * cross_section_nm2 * 1e-18  # nm^2 -> m^2

    # Rouquerol criterion: q * (1 - p) must not decrease over the interval.
    rouq = [q * (1.0 - p) for p, q in zip(ps, qs)]
    if any(b < a for a, b in zip(rouq, rouq[1:])):
        rejections.append("q*(1-p) decreases over interval")

    return BetResult(
        start=start, end=end, n_points=len(ps),
        p_start=ps[0], p_end=ps[-1],
        slope=slope, intercept=intercept,
        monolayer_mol_per_g=n_m, bet_constant=c_const,
        r_squared=r2, area_m2_per_g=area,
        residuals=residuals, rejections=tuple(rejections),
    )


def enumerate_intervals(n, min_points):
    """All contiguous index intervals [start, end] with >= min_points points."""
    for start in range(n):
        for end in range(start + min_points - 1, n):
            yield start, end


def analyze_sample(pressures, quantities, cross_section_nm2,
                   min_points=4, interval=None):
    """Return (selected, candidates). Selected is None when nothing qualifies."""
    n = len(pressures)
    candidates = []
    if interval is not None:
        lo, hi = interval
        idx = [i for i, p in enumerate(pressures) if lo <= p <= hi]
        if len(idx) < min_points:
            return None, []
        candidates.append(
            fit_interval(pressures, quantities, idx[0], idx[-1],
                         cross_section_nm2))
    else:
        for start, end in enumerate_intervals(n, min_points):
            candidates.append(
                fit_interval(pressures, quantities, start, end,
                             cross_section_nm2))

    eligible = [c for c in candidates if c.accepted and c.r_squared is not None]
    if not eligible:
        return None, candidates
    selected = min(
        eligible,
        key=lambda c: (-c.r_squared, -c.n_points, c.p_start),
    )
    return selected, candidates
