"""Balanced two-way random effects ANOVA with interaction."""

from __future__ import annotations

from dataclasses import dataclass

from .data import MeasurementData


@dataclass(frozen=True)
class AnovaRow:
    source: str
    df: int
    ss: float
    ms: float


@dataclass(frozen=True)
class AnovaResult:
    rows: tuple
    total_df: int
    total_ss: float
    ss_check_residual: float
    cell_means: dict
    cell_residuals: dict


def _mean(values):
    return sum(values) / len(values)


def compute_anova(data: MeasurementData) -> AnovaResult:
    a = len(data.parts)
    b = len(data.operators)
    n = data.replicates

    cell_means = {key: _mean(vals) for key, vals in data.cells.items()}
    cell_residuals = {
        key: [v - cell_means[key] for v in vals]
        for key, vals in data.cells.items()
    }

    grand = _mean([v for vals in data.cells.values() for v in vals])
    part_means = {
        p: _mean([cell_means[(p, o)] for o in data.operators]) for p in data.parts
    }
    op_means = {
        o: _mean([cell_means[(p, o)] for p in data.parts]) for o in data.operators
    }

    ss_part = b * n * sum((m - grand) ** 2 for m in part_means.values())
    ss_op = a * n * sum((m - grand) ** 2 for m in op_means.values())
    ss_int = n * sum(
        (cell_means[(p, o)] - part_means[p] - op_means[o] + grand) ** 2
        for p in data.parts
        for o in data.operators
    )
    ss_err = sum(r * r for residuals in cell_residuals.values() for r in residuals)
    ss_total = sum((v - grand) ** 2 for vals in data.cells.values() for v in vals)

    df_part = a - 1
    df_op = b - 1
    df_int = (a - 1) * (b - 1)
    df_err = a * b * (n - 1)
    df_total = a * b * n - 1

    rows = (
        AnovaRow("part", df_part, ss_part, ss_part / df_part),
        AnovaRow("operator", df_op, ss_op, ss_op / df_op),
        AnovaRow("part_by_operator", df_int, ss_int, ss_int / df_int),
        AnovaRow("repeatability", df_err, ss_err, ss_err / df_err),
    )
    ss_check = sum(r.ss for r in rows) - ss_total

    return AnovaResult(
        rows=rows,
        total_df=df_total,
        total_ss=ss_total,
        ss_check_residual=ss_check,
        cell_means=cell_means,
        cell_residuals=cell_residuals,
    )
