"""根据目标总储水量反求地下水位深度（仍使用分层积分模型）。"""

from __future__ import annotations

import math
from dataclasses import dataclass

from .model import SoilProfile
from .storage import compute_storage


class NoSolutionError(ValueError):
    """目标储水量不在 [0, 上限水位] 可达水量区间内。"""


@dataclass
class InvertResult:
    target_storage: float
    water_table_depth: float
    achieved_storage: float
    residual: float
    iterations: int
    bracket: tuple[float, float]
    converged: bool
    tolerance: float

    def to_dict(self) -> dict:
        return {
            "target_storage_cm": self.target_storage,
            "water_table_depth_cm": self.water_table_depth,
            "achieved_storage_cm": self.achieved_storage,
            "residual_cm": self.residual,
            "iterations": self.iterations,
            "bracket_cm": [self.bracket[0], self.bracket[1]],
            "converged": self.converged,
            "tolerance": self.tolerance,
        }


def invert_water_table(
    profile: SoilProfile,
    target_storage: float,
    max_water_table_depth: float,
    tol: float = 1e-7,
    max_iterations: int = 200,
    integration_tol: float = 1e-10,
) -> InvertResult:
    """在 [0, max_water_table_depth] 内反求最浅水位。

    静水平衡下总储水量 S 随水位深度 w 单调递减：w=0 全剖面饱和、
    S 最大；w 越大、不饱和段越厚、S 越小。二分法求使 S(w)=target
    的最浅 w。目标超出端点可达范围时抛出 NoSolutionError，绝不返回
    边界值冒充解。
    """
    target = float(target_storage)
    upper = float(max_water_table_depth)
    if not math.isfinite(target) or target < 0.0:
        raise ValueError("target_storage 必须为非负有限值（cm）")
    if not math.isfinite(upper) or upper <= 0.0:
        raise ValueError("max_water_table_depth 必须为正的有限值（cm）")

    res0 = compute_storage(profile, 0.0, tol=integration_tol)
    res_up = compute_storage(profile, upper, tol=integration_tol)
    if not res0.converged or not res_up.converged:
        raise RuntimeError("端点储水积分未收敛，无法反求")

    s_max = res0.total_storage
    s_min = res_up.total_storage

    if target > s_max + tol or target < s_min - tol:
        raise NoSolutionError(
            f"目标储水量 {target:g} cm 超出可达区间 "
            f"[{s_min:.10g}, {s_max:.10g}] cm"
            f"（水位 {0:g}~{upper:g} cm），无解"
        )

    lo, hi = 0.0, upper
    s_lo, s_hi = s_max, s_min
    iterations = 0
    mid = 0.5 * (lo + hi)
    achieved = math.nan
    for iterations in range(1, max_iterations + 1):
        mid = 0.5 * (lo + hi)
        res = compute_storage(profile, mid, tol=integration_tol)
        if not res.converged:
            raise RuntimeError(f"二分第 {iterations} 步积分未收敛：{res.failures}")
        achieved = res.total_storage
        if abs(achieved - target) <= tol or (hi - lo) <= 1e-12:
            lo = hi = mid
            s_lo = s_hi = achieved
            break
        if achieved > target:
            lo, s_lo = mid, achieved
        else:
            hi, s_hi = mid, achieved
    else:
        raise RuntimeError(f"二分 {max_iterations} 次仍未收敛")

    residual = achieved - target
    converged = abs(residual) <= tol
    return InvertResult(
        target_storage=target,
        water_table_depth=mid,
        achieved_storage=achieved,
        residual=residual,
        iterations=iterations,
        bracket=(lo, hi),
        converged=converged,
        tolerance=tol,
    )
