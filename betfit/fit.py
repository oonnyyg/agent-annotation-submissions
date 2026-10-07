"""BET 线性变换、最小二乘拟合与区间筛选。"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .io import AVOGADRO, Config, Sample


@dataclass
class BetInterval:
    """一个候选连续区间（按排序后的点索引，闭区间）。"""

    start: int
    end: int  # 含
    slope: float = math.nan
    intercept: float = math.nan
    r_squared: float | None = None  # None 表示无法定义
    monolayer_capacity: float = math.nan  # mol/g
    bet_constant: float = math.nan
    surface_area: float = math.nan  # m^2/g
    reasons: list[str] = field(default_factory=list)

    @property
    def n_points(self) -> int:
        return self.end - self.start + 1

    @property
    def accepted(self) -> bool:
        return not self.reasons


@dataclass
class FitResult:
    sample: Sample
    selected: BetInterval | None
    candidates: list[BetInterval]
    residuals: list[tuple[float, float, float, float]]  # (p, y_obs, y_fit, residual)

    @property
    def ok(self) -> bool:
        return self.selected is not None


def _least_squares(xs: list[float], ys: list[float]) -> tuple[float, float, float | None]:
    n = len(xs)
    mx = sum(xs) / n
    my = sum(ys) / n
    sxx = sum((x - mx) ** 2 for x in xs)
    sxy = sum((x - mx) * (y - my) for x, y in zip(xs, ys))
    slope = sxy / sxx  # 同一区间压力互异，sxx > 0
    intercept = my - slope * mx
    ss_tot = sum((y - my) ** 2 for y in ys)
    scale = sum(y * y for y in ys)
    if ss_tot <= 1e-28 * max(scale, 1e-300):
        return slope, intercept, None
    ss_res = sum((y - (slope * x + intercept)) ** 2 for x, y in zip(xs, ys))
    return slope, intercept, 1.0 - ss_res / ss_tot


def _evaluate(sample: Sample, start: int, end: int, sigma_nm2: float) -> BetInterval:
    ps = sample.pressures[start : end + 1]
    qs = sample.loadings[start : end + 1]
    xs = ps
    ys = [p / (q * (1.0 - p)) for p, q in zip(ps, qs)]
    slope, intercept, r2 = _least_squares(xs, ys)

    iv = BetInterval(start=start, end=end, slope=slope, intercept=intercept, r_squared=r2)
    if intercept > 0.0:
        iv.bet_constant = slope / intercept + 1.0
    if slope + intercept > 0.0:
        iv.monolayer_capacity = 1.0 / (slope + intercept)
        iv.surface_area = iv.monolayer_capacity * AVOGADRO * sigma_nm2 * 1e-18

    if intercept <= 0.0:
        iv.reasons.append("截距非正")
    if not iv.monolayer_capacity > 0.0:
        iv.reasons.append("单层容量非正")
    if not iv.bet_constant > 0.0:
        iv.reasons.append("BET 常数非正")
    rouquerol = [q * (1.0 - p) for p, q in zip(ps, qs)]
    if any(b < a for a, b in zip(rouquerol, rouquerol[1:])):
        iv.reasons.append("q*(1-p) 在区间内下降")
    return iv


def enumerate_intervals(sample: Sample, min_points: int, sigma_nm2: float) -> list[BetInterval]:
    n = len(sample.pressures)
    out = []
    for start in range(n):
        for end in range(start + min_points - 1, n):
            out.append(_evaluate(sample, start, end, sigma_nm2))
    return out


def manual_interval(sample: Sample, lower: float, upper: float, min_points: int, sigma_nm2: float) -> BetInterval:
    idx = [i for i, p in enumerate(sample.pressures) if lower <= p <= upper]
    if len(idx) < min_points:
        iv = BetInterval(start=idx[0] if idx else 0, end=idx[-1] if idx else -1)
        iv.reasons.append(f"区间内点数 {len(idx)} 少于 {min_points}")
        return iv
    return _evaluate(sample, idx[0], idx[-1], sigma_nm2)


def select_best(candidates: list[BetInterval]) -> BetInterval | None:
    ok = [c for c in candidates if c.accepted and c.r_squared is not None]
    if not ok:
        return None
    return max(ok, key=lambda c: (c.r_squared, c.n_points, -c.start))


def analyze_sample(sample: Sample, config: Config) -> FitResult:
    spec = config.intervals.get(sample.name)
    if spec is not None:
        chosen = manual_interval(sample, spec.lower, spec.upper, config.min_points, config.cross_section_nm2)
        candidates = [chosen]
        selected = chosen if chosen.accepted else None
    else:
        candidates = enumerate_intervals(sample, config.min_points, config.cross_section_nm2)
        selected = select_best(candidates)

    residuals: list[tuple[float, float, float, float]] = []
    if selected is not None:
        for i in range(selected.start, selected.end + 1):
            p = sample.pressures[i]
            y_obs = p / (sample.loadings[i] * (1.0 - p))
            y_fit = selected.slope * p + selected.intercept
            residuals.append((p, y_obs, y_fit, y_obs - y_fit))
    return FitResult(sample=sample, selected=selected, candidates=candidates, residuals=residuals)
