"""二分类结局的 Meta 分析核心统计模块（仅标准库）。

效应量：实验组相对对照组的优势比（OR），在对数尺度上汇总。
"""
from __future__ import annotations

import csv
import math
from dataclasses import dataclass, field
from pathlib import Path

Z_95 = 1.959963984540054  # 正态近似 95% 区间临界值


class DataError(ValueError):
    """输入数据非法时抛出，消息中定位到具体行。"""


@dataclass(frozen=True)
class Study:
    study_id: str
    exp_events: int
    exp_nonevents: int
    ctl_events: int
    ctl_nonevents: int


@dataclass
class ExcludedStudy:
    study_id: str
    reason: str


@dataclass
class StudyEffect:
    study_id: str
    log_or: float
    variance: float
    odds_ratio: float
    ci_lower: float
    ci_upper: float
    corrected: bool  # 是否做了 0.5 连续性校正
    weight_fixed: float = 0.0
    weight_random: float = 0.0


@dataclass
class Heterogeneity:
    q: float | None
    df: int
    i_squared: float | None
    tau_squared: float | None
    estimable: bool


@dataclass
class PooledResult:
    log_or: float
    se: float
    odds_ratio: float
    ci_lower: float
    ci_upper: float
    heterogeneity: Heterogeneity | None


@dataclass
class LeaveOneOutResult:
    omitted_id: str
    n_remaining: int
    fixed: PooledResult | None
    random: PooledResult | None
    note: str | None = None


@dataclass
class MetaAnalysisResult:
    studies: list[StudyEffect]
    excluded: list[ExcludedStudy]
    fixed: PooledResult
    random: PooledResult
    leave_one_out: list[LeaveOneOutResult] = field(default_factory=list)


REQUIRED_COLUMNS = (
    "study_id",
    "exp_events",
    "exp_nonevents",
    "ctl_events",
    "ctl_nonevents",
)


def _parse_count(raw: str, row_no: int, column: str) -> int:
    text = raw.strip()
    try:
        value = int(text)
    except ValueError:
        raise DataError(
            f"第 {row_no} 行：列 {column} 的值 {text!r} 不是整数"
        ) from None
    if value < 0:
        raise DataError(f"第 {row_no} 行：列 {column} 的计数 {value} 为负数")
    return value


def load_studies(path: str | Path) -> list[Study]:
    """读取 CSV，校验计数合法性并检查重复研究编号。"""
    studies: list[Study] = []
    seen: dict[str, int] = {}
    with open(path, newline="", encoding="utf-8-sig") as fh:
        reader = csv.DictReader(fh)
        missing = [c for c in REQUIRED_COLUMNS if c not in (reader.fieldnames or [])]
        if missing:
            raise DataError(f"CSV 缺少必需列：{', '.join(missing)}")
        for row_no, row in enumerate(reader, start=2):
            study_id = (row.get("study_id") or "").strip()
            if not study_id:
                raise DataError(f"第 {row_no} 行：研究编号为空")
            if study_id in seen:
                raise DataError(
                    f"第 {row_no} 行：研究编号 {study_id!r} 与第 {seen[study_id]} 行重复"
                )
            seen[study_id] = row_no
            counts = {
                col: _parse_count(row[col], row_no, col)
                for col in REQUIRED_COLUMNS[1:]
            }
            if counts["exp_events"] + counts["exp_nonevents"] <= 0:
                raise DataError(f"第 {row_no} 行：实验组总人数必须为正")
            if counts["ctl_events"] + counts["ctl_nonevents"] <= 0:
                raise DataError(f"第 {row_no} 行：对照组总人数必须为正")
            studies.append(Study(study_id=study_id, **counts))
    return studies


def compute_effect(study: Study) -> StudyEffect | ExcludedStudy:
    """计算单项研究的对数 OR 与方差；无法定义效应时返回排除记录。"""
    a, b = study.exp_events, study.exp_nonevents
    c, d = study.ctl_events, study.ctl_nonevents
    if (a == 0 and c == 0) or (b == 0 and d == 0):
        reason = (
            "两组均无事件，优势比不可定义"
            if a == 0 and c == 0
            else "两组均全部发生事件，优势比不可定义"
        )
        return ExcludedStudy(study.study_id, reason)
    corrected = 0 in (a, b, c, d)
    if corrected:
        a, b, c, d = a + 0.5, b + 0.5, c + 0.5, d + 0.5
    log_or = math.log((a * d) / (b * c))
    variance = 1.0 / a + 1.0 / b + 1.0 / c + 1.0 / d
    half = Z_95 * math.sqrt(variance)
    return StudyEffect(
        study_id=study.study_id,
        log_or=log_or,
        variance=variance,
        odds_ratio=math.exp(log_or),
        ci_lower=math.exp(log_or - half),
        ci_upper=math.exp(log_or + half),
        corrected=corrected,
    )


def _dl_tau_squared(effects: list[StudyEffect]) -> tuple[float, float, int]:
    """DerSimonian-Laird 研究间方差；返回 (tau^2, Q, df)。"""
    k = len(effects)
    df = k - 1
    weights = [1.0 / e.variance for e in effects]
    pooled = sum(w * e.log_or for w, e in zip(weights, effects)) / sum(weights)
    q = sum(w * (e.log_or - pooled) ** 2 for w, e in zip(weights, effects))
    c = sum(weights) - sum(w * w for w in weights) / sum(weights)
    tau2 = max(0.0, (q - df) / c) if c > 0 else 0.0
    return tau2, q, df


def _pool(effects: list[StudyEffect], variances: list[float]) -> tuple[float, float]:
    weights = [1.0 / v for v in variances]
    total = sum(weights)
    pooled = sum(w * e.log_or for w, e in zip(weights, effects)) / total
    se = math.sqrt(1.0 / total)
    return pooled, se


def _pooled_result(log_or: float, se: float, heterogeneity: Heterogeneity | None) -> PooledResult:
    half = Z_95 * se
    return PooledResult(
        log_or=log_or,
        se=se,
        odds_ratio=math.exp(log_or),
        ci_lower=math.exp(log_or - half),
        ci_upper=math.exp(log_or + half),
        heterogeneity=heterogeneity,
    )


def _pool_fixed(effects: list[StudyEffect]) -> PooledResult:
    log_or, se = _pool(effects, [e.variance for e in effects])
    return _pooled_result(log_or, se, None)


def _pool_random(effects: list[StudyEffect]) -> PooledResult:
    tau2, q, df = _dl_tau_squared(effects)
    i2 = max(0.0, (q - df) / q) * 100.0 if q > 0 else 0.0
    log_or, se = _pool(effects, [e.variance + tau2 for e in effects])
    heterogeneity = Heterogeneity(q=q, df=df, i_squared=i2, tau_squared=tau2, estimable=True)
    return _pooled_result(log_or, se, heterogeneity)


def _assign_weights(effects: list[StudyEffect], tau2: float) -> None:
    fixed_total = sum(1.0 / e.variance for e in effects)
    random_total = sum(1.0 / (e.variance + tau2) for e in effects)
    for e in effects:
        e.weight_fixed = (1.0 / e.variance) / fixed_total * 100.0
        e.weight_random = (1.0 / (e.variance + tau2)) / random_total * 100.0


def _leave_one_out(effects: list[StudyEffect]) -> list[LeaveOneOutResult]:
    results: list[LeaveOneOutResult] = []
    for omitted in effects:
        remaining = [e for e in effects if e is not omitted]
        n = len(remaining)
        if n == 0:
            results.append(
                LeaveOneOutResult(omitted.study_id, 0, None, None, note="移除后无剩余研究，无法汇总")
            )
            continue
        fixed = _pool_fixed(remaining)
        if n == 1:
            random = _pooled_result(
                fixed.log_or,
                fixed.se,
                Heterogeneity(q=None, df=0, i_squared=None, tau_squared=None, estimable=False),
            )
            note = "仅剩一项研究，异质性不可估计"
        else:
            random = _pool_random(remaining)
            note = None
        results.append(LeaveOneOutResult(omitted.study_id, n, fixed, random, note=note))
    return results


def run_meta_analysis(studies: list[Study]) -> MetaAnalysisResult:
    effects: list[StudyEffect] = []
    excluded: list[ExcludedStudy] = []
    for study in studies:
        outcome = compute_effect(study)
        if isinstance(outcome, ExcludedStudy):
            excluded.append(outcome)
        else:
            effects.append(outcome)
    if not effects:
        raise DataError("没有可纳入分析的研究（全部研究均被排除）")
    fixed = _pool_fixed(effects)
    if len(effects) == 1:
        random = _pooled_result(
            fixed.log_or,
            fixed.se,
            Heterogeneity(q=None, df=0, i_squared=None, tau_squared=None, estimable=False),
        )
        tau2 = 0.0
    else:
        random = _pool_random(effects)
        tau2 = random.heterogeneity.tau_squared
    _assign_weights(effects, tau2)
    loo = _leave_one_out(effects)
    return MetaAnalysisResult(
        studies=effects, excluded=excluded, fixed=fixed, random=random, leave_one_out=loo
    )
