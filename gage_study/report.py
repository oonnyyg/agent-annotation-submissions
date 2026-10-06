"""Writers for ANOVA / variance component CSV and JSON outputs."""

from __future__ import annotations

import csv
import json
import os

from .anova import AnovaResult
from .data import MeasurementData
from .variance import VarianceComponents


def _fmt(value):
    if value is None:
        return ""
    return repr(float(value))


def write_anova_csv(path: str, anova: AnovaResult) -> None:
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(["source", "df", "ss", "ms"])
        for row in anova.rows:
            writer.writerow([row.source, row.df, _fmt(row.ss), _fmt(row.ms)])
        writer.writerow(["total", anova.total_df, _fmt(anova.total_ss), ""])


def write_variance_csv(path: str, vc: VarianceComponents) -> None:
    raw = {
        "part": vc.part,
        "operator": vc.operator,
        "part_by_operator": vc.interaction,
        "repeatability": vc.repeatability,
    }
    used = {
        "part": max(0.0, vc.part),
        "operator": max(0.0, vc.operator),
        "part_by_operator": max(0.0, vc.interaction),
        "repeatability": max(0.0, vc.repeatability),
        "reproducibility": vc.reproducibility,
        "gage": vc.gage,
        "total": vc.total,
    }
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "component",
                "variance_estimate",
                "variance_used",
                "contribution_pct",
                "stddev_pct",
            ]
        )
        for name in (
            "part",
            "operator",
            "part_by_operator",
            "repeatability",
            "reproducibility",
            "gage",
            "total",
        ):
            writer.writerow(
                [
                    name,
                    _fmt(raw.get(name)),
                    _fmt(used[name]),
                    _fmt(vc.contribution_pct[name]),
                    _fmt(vc.stddev_pct[name]),
                ]
            )


def write_means_json(path: str, data: MeasurementData, anova: AnovaResult) -> None:
    cells = []
    for key in sorted(anova.cell_means):
        part, operator = key
        cells.append(
            {
                "part": part,
                "operator": operator,
                "mean": anova.cell_means[key],
                "residuals": list(anova.cell_residuals[key]),
            }
        )
    payload = {
        "parts": list(data.parts),
        "operators": list(data.operators),
        "replicates": data.replicates,
        "cells": cells,
    }
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=2, ensure_ascii=False)
        fh.write("\n")


def write_diagnostics_json(
    path: str,
    data: MeasurementData,
    anova: AnovaResult,
    vc: VarianceComponents,
) -> None:
    warnings = list(vc.warnings)
    if abs(anova.ss_check_residual) > 1e-9 * max(1.0, abs(anova.total_ss)):
        warnings.append(
            "sum of source SS does not match total SS: "
            f"residual={anova.ss_check_residual:.6g}"
        )
    payload = {
        "n_parts": len(data.parts),
        "n_operators": len(data.operators),
        "replicates_per_cell": data.replicates,
        "n_observations": len(data.parts) * len(data.operators) * data.replicates,
        "ss_decomposition_residual": anova.ss_check_residual,
        "negative_variance_estimates": [
            name
            for name, value in (
                ("part", vc.part),
                ("operator", vc.operator),
                ("part_by_operator", vc.interaction),
                ("repeatability", vc.repeatability),
            )
            if value < 0
        ],
        "warnings": warnings,
    }
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(payload, fh, indent=2, ensure_ascii=False)
        fh.write("\n")


def write_all(
    out_dir: str,
    data: MeasurementData,
    anova: AnovaResult,
    vc: VarianceComponents,
) -> dict:
    os.makedirs(out_dir, exist_ok=True)
    paths = {
        "anova": os.path.join(out_dir, "anova.csv"),
        "variance_components": os.path.join(out_dir, "variance_components.csv"),
        "means": os.path.join(out_dir, "means.json"),
        "diagnostics": os.path.join(out_dir, "diagnostics.json"),
    }
    write_anova_csv(paths["anova"], anova)
    write_variance_csv(paths["variance_components"], vc)
    write_means_json(paths["means"], data, anova)
    write_diagnostics_json(paths["diagnostics"], data, anova, vc)
    return paths
