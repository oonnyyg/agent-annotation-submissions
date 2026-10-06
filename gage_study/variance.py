"""Method-of-moments variance components from expected mean squares."""

from __future__ import annotations

import math
from dataclasses import dataclass

from .anova import AnovaResult
from .data import MeasurementData


@dataclass(frozen=True)
class VarianceComponents:
    part: float
    operator: float
    interaction: float
    repeatability: float
    warnings: tuple
    reproducibility: float
    gage: float
    total: float
    contribution_pct: dict
    stddev_pct: dict


def compute_variance_components(
    data: MeasurementData, anova: AnovaResult
) -> VarianceComponents:
    a = len(data.parts)
    b = len(data.operators)
    n = data.replicates
    ms = {row.source: row.ms for row in anova.rows}

    var_rep = ms["repeatability"]
    var_int = (ms["part_by_operator"] - ms["repeatability"]) / n
    var_op = (ms["operator"] - ms["part_by_operator"]) / (a * n)
    var_part = (ms["part"] - ms["part_by_operator"]) / (b * n)

    raw = {
        "part": var_part,
        "operator": var_op,
        "part_by_operator": var_int,
        "repeatability": var_rep,
    }
    warnings = tuple(
        f"negative variance estimate for {name}: {value:.6g}; "
        "kept as reported but floored to zero in summaries"
        for name, value in raw.items()
        if value < 0
    )
    used = {name: max(0.0, value) for name, value in raw.items()}

    reproducibility = used["operator"] + used["part_by_operator"]
    gage = reproducibility + used["repeatability"]
    total = gage + used["part"]

    names = (
        "part",
        "operator",
        "part_by_operator",
        "repeatability",
        "reproducibility",
        "gage",
        "total",
    )
    values = {
        "part": used["part"],
        "operator": used["operator"],
        "part_by_operator": used["part_by_operator"],
        "repeatability": used["repeatability"],
        "reproducibility": reproducibility,
        "gage": gage,
        "total": total,
    }
    if total > 0:
        contribution = {k: values[k] / total * 100.0 for k in names}
        stddev_pct = {
            k: math.sqrt(values[k]) / math.sqrt(total) * 100.0 for k in names
        }
    else:
        contribution = {k: None for k in names}
        stddev_pct = {k: None for k in names}

    return VarianceComponents(
        part=var_part,
        operator=var_op,
        interaction=var_int,
        repeatability=var_rep,
        warnings=warnings,
        reproducibility=reproducibility,
        gage=gage,
        total=total,
        contribution_pct=contribution,
        stddev_pct=stddev_pct,
    )
