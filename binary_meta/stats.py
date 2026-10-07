"""Effect size computation, pooling, heterogeneity and leave-one-out analysis.

All pooling is done on the log odds-ratio scale with normal-approximation
95% confidence intervals; odds-ratio intervals are obtained by exponentiating.
"""

from __future__ import annotations

import math
from dataclasses import dataclass

from .data import Study

Z_95 = 1.959963984540054
CONTINUITY_CORRECTION = 0.5


@dataclass(frozen=True)
class ExcludedStudy:
    study_id: str
    reason: str


@dataclass(frozen=True)
class StudyEffect:
    study_id: str
    log_or: float
    variance: float
    corrected: bool

    @property
    def se(self) -> float:
        return math.sqrt(self.variance)

    @property
    def or_ci(self) -> tuple[float, float, float]:
        lo = self.log_or - Z_95 * self.se
        hi = self.log_or + Z_95 * self.se
        return (math.exp(self.log_or), math.exp(lo), math.exp(hi))


@dataclass(frozen=True)
class PooledResult:
    log_or: float
    se: float
    ci_lo: float
    ci_hi: float

    @property
    def or_ci(self) -> tuple[float, float, float]:
        return (math.exp(self.log_or), math.exp(self.ci_lo), math.exp(self.ci_hi))


@dataclass(frozen=True)
class Heterogeneity:
    q: float | None
    df: int
    i2: float | None
    tau2: float | None


@dataclass(frozen=True)
class MetaResult:
    k: int
    fixed: PooledResult | None
    random: PooledResult | None
    heterogeneity: Heterogeneity
    fixed_weights: tuple[float, ...]
    random_weights: tuple[float, ...]


def compute_effect(study: Study) -> StudyEffect | ExcludedStudy:
    """Log odds ratio and variance for one study.

    Studies with no events in either group, or events in every subject of
    both groups, are excluded because the odds ratio is undefined. Any
    other study with a zero cell gets a 0.5 continuity correction.
    """
    a, b = study.e_events, study.e_nonevents
    c, d = study.c_events, study.c_nonevents
    if a == 0 and c == 0:
        return ExcludedStudy(study.study_id, "no events in either group; odds ratio undefined")
    if b == 0 and d == 0:
        return ExcludedStudy(
            study.study_id, "all subjects have events in both groups; odds ratio undefined"
        )
    corrected = 0 in (a, b, c, d)
    if corrected:
        a += CONTINUITY_CORRECTION
        b += CONTINUITY_CORRECTION
        c += CONTINUITY_CORRECTION
        d += CONTINUITY_CORRECTION
    log_or = math.log((a * d) / (b * c))
    variance = 1.0 / a + 1.0 / b + 1.0 / c + 1.0 / d
    return StudyEffect(study.study_id, log_or, variance, corrected)


def _pool(log_ors: list[float], variances: list[float], tau2: float) -> PooledResult:
    weights = [1.0 / (v + tau2) for v in variances]
    total_w = sum(weights)
    mean = sum(w * y for w, y in zip(weights, log_ors)) / total_w
    se = math.sqrt(1.0 / total_w)
    return PooledResult(mean, se, mean - Z_95 * se, mean + Z_95 * se)


def _dl_tau2(log_ors: list[float], variances: list[float], q: float) -> float:
    k = len(log_ors)
    weights = [1.0 / v for v in variances]
    sum_w = sum(weights)
    sum_w2 = sum(w * w for w in weights)
    denom = sum_w - sum_w2 / sum_w
    if denom <= 0:
        return 0.0
    return max(0.0, (q - (k - 1)) / denom)


def meta_analyze(effects: list[StudyEffect]) -> MetaResult:
    """Fixed-effect and DerSimonian-Laird random-effects pooling."""
    k = len(effects)
    if k == 0:
        return MetaResult(0, None, None, Heterogeneity(None, 0, None, None), (), ())
    log_ors = [e.log_or for e in effects]
    variances = [e.variance for e in effects]
    fixed = _pool(log_ors, variances, 0.0)
    if k == 1:
        het = Heterogeneity(None, 0, None, None)
        random = PooledResult(fixed.log_or, fixed.se, fixed.ci_lo, fixed.ci_hi)
        return MetaResult(1, fixed, random, het, (100.0,), (100.0,))
    q = sum((y - fixed.log_or) ** 2 / v for y, v in zip(log_ors, variances))
    tau2 = _dl_tau2(log_ors, variances, q)
    i2 = max(0.0, (q - (k - 1)) / q) * 100.0 if q > 0 else 0.0
    random = _pool(log_ors, variances, tau2)
    fw = [100.0 * (1.0 / v) / sum(1.0 / v2 for v2 in variances) for v in variances]
    rw_raw = [1.0 / (v + tau2) for v in variances]
    rw = [100.0 * w / sum(rw_raw) for w in rw_raw]
    return MetaResult(k, fixed, random, Heterogeneity(q, k - 1, i2, tau2), tuple(fw), tuple(rw))


def leave_one_out(effects: list[StudyEffect]) -> list[tuple[str, MetaResult]]:
    """Re-run the full analysis omitting each study in turn.

    Weights and heterogeneity are recomputed from the remaining studies;
    the full-data random-effects weights are never reused.
    """
    results: list[tuple[str, MetaResult]] = []
    for i, effect in enumerate(effects):
        remaining = effects[:i] + effects[i + 1 :]
        results.append((effect.study_id, meta_analyze(remaining)))
    return results
