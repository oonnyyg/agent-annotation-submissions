"""Balanced two-factor random-effects ANOVA with interaction."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .data import Observation


@dataclass(frozen=True)
class AnovaRow:
    source: str
    df: int
    ss: float
    ms: float


@dataclass
class StudyResult:
    parts: list[str]
    operators: list[str]
    n_replicates: int
    anova: list[AnovaRow]
    variance_components: dict[str, float]
    warnings: list[str] = field(default_factory=list)
    cell_means: dict[tuple[str, str], float] = field(default_factory=dict)
    residuals: list[tuple[str, str, int, float]] = field(default_factory=list)
    summary: dict[str, dict] = field(default_factory=dict)
    ss_check: dict = field(default_factory=dict)


def _mean(values: list[float]) -> float:
    return math.fsum(values) / len(values)


def analyze(observations: list[Observation], parts: list[str],
            operators: list[str], n_replicates: int) -> StudyResult:
    a = len(parts)
    b = len(operators)
    n = n_replicates

    cell_values: dict[tuple[str, str], list[float]] = {}
    for obs in observations:
        cell_values.setdefault((obs.part, obs.operator), []).append(obs.value)
    cell_means = {key: _mean(vals) for key, vals in cell_values.items()}

    grand = _mean([o.value for o in observations])
    part_means = {p: _mean([cell_means[(p, op)] for op in operators]) for p in parts}
    op_means = {op: _mean([cell_means[(p, op)] for p in parts]) for op in operators}

    ss_part = b * n * math.fsum((m - grand) ** 2 for m in part_means.values())
    ss_op = a * n * math.fsum((m - grand) ** 2 for m in op_means.values())
    ss_inter = n * math.fsum(
        (cell_means[(p, op)] - part_means[p] - op_means[op] + grand) ** 2
        for p in parts for op in operators
    )
    ss_error = math.fsum(
        (obs.value - cell_means[(obs.part, obs.operator)]) ** 2
        for obs in observations
    )
    ss_total = math.fsum((obs.value - grand) ** 2 for obs in observations)

    df_part, df_op = a - 1, b - 1
    df_inter = df_part * df_op
    df_error = a * b * (n - 1)
    df_total = a * b * n - 1

    anova = [
        AnovaRow("part", df_part, ss_part, ss_part / df_part),
        AnovaRow("operator", df_op, ss_op, ss_op / df_op),
        AnovaRow("part_operator", df_inter, ss_inter, ss_inter / df_inter),
        AnovaRow("repeatability", df_error, ss_error, ss_error / df_error),
        AnovaRow("total", df_total, ss_total, math.nan),
    ]

    ms_part, ms_op, ms_inter, ms_error = (r.ms for r in anova[:4])
    components = {
        "part": (ms_part - ms_inter) / (b * n),
        "operator": (ms_op - ms_inter) / (a * n),
        "part_operator": (ms_inter - ms_error) / n,
        "repeatability": ms_error,
    }

    warnings: list[str] = []
    for name, value in components.items():
        if value < 0:
            warnings.append(
                f"negative variance estimate for {name}: {value!r}; "
                "kept as-is in output but treated as 0 in summaries"
            )

    residuals = [
        (obs.part, obs.operator, obs.replicate,
         obs.value - cell_means[(obs.part, obs.operator)])
        for obs in observations
    ]

    result = StudyResult(
        parts=parts, operators=operators, n_replicates=n,
        anova=anova, variance_components=components, warnings=warnings,
        cell_means=cell_means, residuals=residuals,
    )
    result.summary = _summarize(components)
    ss_sum = ss_part + ss_op + ss_inter + ss_error
    result.ss_check = {
        "sum_of_sources": ss_sum,
        "total": ss_total,
        "difference": ss_sum - ss_total,
        "ok": math.isclose(ss_sum, ss_total, rel_tol=1e-9, abs_tol=1e-12),
    }
    return result


def _summarize(components: dict[str, float]) -> dict[str, dict]:
    vc = {k: max(v, 0.0) for k, v in components.items()}
    reproducibility = vc["operator"] + vc["part_operator"]
    gage = reproducibility + vc["repeatability"]
    total = gage + vc["part"]

    rows: dict[str, dict] = {}
    entries = {
        "gage": gage,
        "repeatability": vc["repeatability"],
        "reproducibility": reproducibility,
        "operator": vc["operator"],
        "part_operator": vc["part_operator"],
        "part": vc["part"],
        "total": total,
    }
    for name, var in entries.items():
        if total > 0:
            pct_var = 100.0 * var / total
            pct_sd = 100.0 * math.sqrt(var / total)
        else:
            pct_var = None
            pct_sd = None
        rows[name] = {
            "variance": var,
            "stddev": math.sqrt(var),
            "percent_variance": pct_var,
            "percent_stddev": pct_sd,
        }
    return rows
