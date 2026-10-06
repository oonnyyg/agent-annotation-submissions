"""沿深度积分含水率，得到分层与总储水厚度（cm）。

积分在土层边界与地下水位边界处分段；段内使用自适应 Simpson 求积，
不以层中心含水率乘厚度代替积分。
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .model import SoilProfile, water_content_at


class IntegrationError(RuntimeError):
    """自适应积分在允许的递归深度内未收敛。"""


def _simpson(f, a: float, fa: float, fm: float, fb: float) -> float:
    return (b - a) / 6.0 * (fa + 4.0 * fm + fb)


def _adaptive_simpson(
    f, a, b, fa, fm, fb, whole, tol, depth
) -> tuple[float, float, int]:
    """返回 (积分值, 误差估计, 剩余递归层级)。"""
    m = 0.5 * (a + b)
    lm = 0.5 * (a + m)
    rm = 0.5 * (m + b)
    flm = f(lm)
    frm = f(rm)
    left = _simpson(f, a, m, fa, flm, fm)
    right = _simpson(f, m, b, fm, frm, fb)
    refined = left + right
    err = abs(refined - whole) / 15.0
    if depth <= 0:
        if err > tol:
            raise IntegrationError(
                f"区间 [{a:g}, {b:g}] 积分未收敛，误差估计 {err:g} > 容差 {tol:g}"
            )
        return refined + (refined - whole) / 15.0, err, depth
    if err <= tol:
        return refined + (refined - whole) / 15.0, err, depth
    lval, lerr, ld = _adaptive_simpson(
        f, a, m, fa, flm, fm, left, tol / 2.0, depth - 1
    )
    rval, rerr, rd = _adaptive_simpson(
        f, m, b, fm, frm, fb, right, tol / 2.0, depth - 1
    )
    return lval + rval, lerr + rerr, min(ld, rd)


def integrate_segment(f, a: float, b: float, tol: float, max_depth: int = 30):
    if b <= a:
        return 0.0, 0.0, max_depth
    fa = f(a)
    fb = f(b)
    fm = f(0.5 * (a + b))
    whole = _simpson(f, a, b, fa, fm, fb)
    return _adaptive_simpson(f, a, b, fa, fm, fb, whole, tol, max_depth)


def _split_segments(
    profile: SoilProfile, water_table_depth: float
) -> list[tuple[float, float]]:
    """按层界与水位边界切分积分区间（跳过零长度段）。"""
    cuts = sorted(
        {
            0.0,
            profile.total_depth,
            max(0.0, min(water_table_depth, profile.total_depth)),
            *profile.boundaries[1:-1],
        }
    )
    return [
        (cuts[i], cuts[i + 1])
        for i in range(len(cuts) - 1)
        if cuts[i + 1] > cuts[i]
    ]


@dataclass
class LayerStorage:
    index: int
    top: float
    bottom: float
    storage: float
    error: float


@dataclass
class StorageResult:
    water_table_depth: float
    layers: list[LayerStorage]
    total_storage: float
    total_error: float
    tolerance: float
    converged: bool
    failures: list[str] = field(default_factory=list)

    def to_dict(self) -> dict:
        return {
            "water_table_depth_cm": self.water_table_depth,
            "total_storage_cm": self.total_storage,
            "total_error": self.total_error,
            "tolerance": self.tolerance,
            "converged": self.converged,
            "failures": self.failures,
            "layers": [
                {
                    "index": ls.index,
                    "top_cm": ls.top,
                    "bottom_cm": ls.bottom,
                    "storage_cm": ls.storage,
                    "error": ls.error,
                }
                for ls in self.layers
            ],
        }


def compute_storage(
    profile: SoilProfile,
    water_table_depth: float,
    tol: float = 1e-9,
    max_depth: int = 30,
) -> StorageResult:
    """计算给定水位下各层与全剖面的储水厚度（cm）。"""
    if not (isinstance(tol, (int, float)) and math.isfinite(tol) and tol > 0.0):
        raise ValueError("tol 必须为正的有限值")
    if not (
        isinstance(water_table_depth, (int, float))
        and math.isfinite(water_table_depth)
    ):
        raise ValueError("water_table_depth 必须为有限值")
    if water_table_depth < 0.0:
        raise ValueError("water_table_depth 不能为负")

    segments = _split_segments(profile, water_table_depth)
    total_len = profile.total_depth

    per_layer_storage = [0.0] * len(profile.layers)
    per_layer_error = [0.0] * len(profile.layers)
    failures: list[str] = []

    for a, b in segments:
        idx = profile.layer_index_at(0.5 * (a + b))
        seg_tol = tol * (b - a) / total_len

        def f(z: float, _p=profile, _w=water_table_depth) -> float:
            return water_content_at(_p, z, _w)

        try:
            value, err, _ = integrate_segment(f, a, b, seg_tol, max_depth)
        except IntegrationError as exc:
            failures.append(str(exc))
            value, err = math.nan, math.inf
        per_layer_storage[idx] += value
        per_layer_error[idx] += err

    layer_results = [
        LayerStorage(
            index=i,
            top=profile.boundaries[i],
            bottom=profile.boundaries[i + 1],
            storage=per_layer_storage[i],
            error=per_layer_error[i],
        )
        for i in range(len(profile.layers))
    ]
    if failures:
        total_storage = math.nan
        total_error = math.inf
        converged = False
    else:
        total_storage = sum(per_layer_storage)
        total_error = sum(per_layer_error)
        converged = total_error <= tol * max(1, len(segments))
    return StorageResult(
        water_table_depth=float(water_table_depth),
        layers=layer_results,
        total_storage=total_storage,
        total_error=total_error,
        tolerance=tol,
        converged=converged,
        failures=failures,
    )
