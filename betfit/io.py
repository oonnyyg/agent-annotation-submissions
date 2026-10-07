"""CSV 数据与 JSON 配置的读取及校验。"""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass, field
from pathlib import Path

AVOGADRO = 6.02214076e23  # mol^-1


class BetInputError(ValueError):
    """输入数据或配置不合法。"""


@dataclass
class Sample:
    name: str
    pressures: list[float]  # 相对压力 p/p0，0 < p < 1
    loadings: list[float]  # 单位质量吸附量，mol/g


@dataclass
class IntervalSpec:
    """手动指定的闭区间（按相对压力）。"""

    lower: float
    upper: float


@dataclass
class Config:
    cross_section_nm2: float  # 分子截面积，nm^2
    min_points: int = 4
    intervals: dict[str, IntervalSpec] = field(default_factory=dict)


def _check_finite(value: float, where: str) -> float:
    if not math.isfinite(value):
        raise BetInputError(f"{where}: 数值非有限 ({value!r})")
    return value


def load_samples(csv_path: str | Path) -> list[Sample]:
    """读取 CSV：首列相对压力，其后每列为一个样品的吸附量 (mol/g)。

    表头第一列任意（如 pressure），其余列为样品名。
    """
    path = Path(csv_path)
    with path.open(newline="", encoding="utf-8-sig") as fh:
        rows = list(csv.reader(fh))
    if len(rows) < 2:
        raise BetInputError(f"{path}: 至少需要表头和一行数据")
    header = rows[0]
    if len(header) < 2:
        raise BetInputError(f"{path}: 至少需要压力列和一个样品列")
    names = [h.strip() or f"sample_{i}" for i, h in enumerate(header[1:], start=1)]

    columns: list[list[tuple[int, float]]] = [[] for _ in names]
    pressures: list[tuple[int, float]] = []
    for lineno, row in enumerate(rows[1:], start=2):
        if not row or all(not cell.strip() for cell in row):
            continue
        if len(row) != len(header):
            raise BetInputError(f"{path}:{lineno}: 列数 {len(row)} 与表头 {len(header)} 不一致")
        try:
            p = float(row[0])
        except ValueError as exc:
            raise BetInputError(f"{path}:{lineno}: 压力无法解析为数值: {row[0]!r}") from exc
        _check_finite(p, f"{path}:{lineno} 压力")
        if not 0.0 < p < 1.0:
            raise BetInputError(f"{path}:{lineno}: 相对压力 {p} 不在 (0, 1) 内")
        pressures.append((lineno, p))
        for col, cell in enumerate(row[1:]):
            try:
                q = float(cell)
            except ValueError as exc:
                raise BetInputError(
                    f"{path}:{lineno} 样品 {names[col]}: 吸附量无法解析: {cell!r}"
                ) from exc
            _check_finite(q, f"{path}:{lineno} 样品 {names[col]} 吸附量")
            columns[col].append((lineno, q))

    samples = []
    for name, col in zip(names, columns):
        pairs = sorted(zip([p for _, p in pressures], [q for _, q in col]), key=lambda t: t[0])
        ps = [p for p, _ in pairs]
        for i in range(1, len(ps)):
            if ps[i] == ps[i - 1]:
                raise BetInputError(f"{path} 样品 {name}: 重复压力 {ps[i]}")
        samples.append(Sample(name=name, pressures=ps, loadings=[q for _, q in pairs]))
    return samples


def load_config(json_path: str | Path) -> Config:
    """读取 JSON 配置。

    字段：
      cross_section_nm2: 分子截面积 (nm^2)，必须为正
      min_points: 每个区间最少点数，默认 4
      intervals: {样品名: {"lower": x, "upper": y}} 手动闭区间（可选）
    """
    path = Path(json_path)
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise BetInputError(f"{path}: JSON 解析失败: {exc}") from exc
    if not isinstance(raw, dict):
        raise BetInputError(f"{path}: 配置必须是 JSON 对象")

    try:
        sigma = float(raw["cross_section_nm2"])
    except (KeyError, TypeError, ValueError) as exc:
        raise BetInputError(f"{path}: 缺少有效的 cross_section_nm2") from exc
    _check_finite(sigma, f"{path} cross_section_nm2")
    if sigma <= 0:
        raise BetInputError(f"{path}: cross_section_nm2 必须为正")

    min_points = int(raw.get("min_points", 4))
    if min_points < 4:
        raise BetInputError(f"{path}: min_points 至少为 4")

    intervals: dict[str, IntervalSpec] = {}
    for name, spec in (raw.get("intervals") or {}).items():
        try:
            lower = float(spec["lower"])
            upper = float(spec["upper"])
        except (KeyError, TypeError, ValueError) as exc:
            raise BetInputError(f"{path}: 样品 {name} 的区间需要 lower/upper 数值") from exc
        _check_finite(lower, f"{path} 样品 {name} lower")
        _check_finite(upper, f"{path} 样品 {name} upper")
        if not 0.0 < lower < upper < 1.0:
            raise BetInputError(f"{path}: 样品 {name} 区间需满足 0 < lower < upper < 1")
        intervals[name] = IntervalSpec(lower=lower, upper=upper)
    return Config(cross_section_nm2=sigma, min_points=min_points, intervals=intervals)
