"""JSON/CSV input parsing and report serialization."""
from __future__ import annotations

import csv
import json
from typing import List

from .model import Experiment, SiteClass
from .solver import EquilibriumResult


def load_sites(path: str) -> List[SiteClass]:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, dict) or not isinstance(data.get("sites"), list):
        raise ValueError("sites JSON must be an object with a 'sites' list")
    sites = []
    for i, entry in enumerate(data["sites"]):
        try:
            sites.append(
                SiteClass(
                    name=str(entry["name"]),
                    total=entry["total"],
                    kd_a=entry["kd_a"],
                    kd_b=entry["kd_b"],
                )
            )
        except (KeyError, TypeError) as exc:
            raise ValueError(f"sites[{i}] missing/invalid field: {exc}") from exc
    if not sites:
        raise ValueError("sites list must not be empty")
    return sites


def load_experiments(path: str) -> List[Experiment]:
    experiments = []
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        required = {"total_a", "total_b"}
        if reader.fieldnames is None or not required <= set(reader.fieldnames):
            raise ValueError("CSV must have columns: total_a,total_b")
        for rownum, row in enumerate(reader, start=2):
            try:
                experiments.append(
                    Experiment(float(row["total_a"]), float(row["total_b"]))
                )
            except (ValueError, TypeError) as exc:
                raise ValueError(f"CSV row {rownum}: {exc}") from exc
    if not experiments:
        raise ValueError("CSV contains no experiments")
    return experiments


def result_rows(result: EquilibriumResult) -> List[dict]:
    rows = []
    for c in result.classes:
        rows.append(
            {
                "total_a": result.experiment.total_a,
                "total_b": result.experiment.total_b,
                "class": c.name,
                "frac_free": c.frac_free,
                "occ_a": c.occ_a,
                "occ_b": c.occ_b,
                "bound_a": c.bound_a,
                "bound_b": c.bound_b,
                "free_a": result.free_a,
                "free_b": result.free_b,
            }
        )
    return rows


def write_results_csv(path: str, results: List[EquilibriumResult]) -> None:
    fields = [
        "total_a", "total_b", "class", "frac_free", "occ_a", "occ_b",
        "bound_a", "bound_b", "free_a", "free_b",
    ]
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(fh, fieldnames=fields)
        writer.writeheader()
        for r in results:
            writer.writerows(result_rows(r))


def result_report(result: EquilibriumResult) -> dict:
    return {
        "input": {
            "total_a": result.experiment.total_a,
            "total_b": result.experiment.total_b,
        },
        "free": {"free_a": result.free_a, "free_b": result.free_b},
        "bound": {"bound_a": result.bound_a, "bound_b": result.bound_b},
        "mass_balance": {
            "a": result.free_a + result.bound_a,
            "b": result.free_b + result.bound_b,
        },
        "residuals": {
            "residual_a": result.residual_a,
            "residual_b": result.residual_b,
        },
        "converged": result.converged,
        "iterations": result.iterations,
        "classes": [
            {
                "name": c.name,
                "total": c.total,
                "frac_free": c.frac_free,
                "occ_a": c.occ_a,
                "occ_b": c.occ_b,
                "bound_a": c.bound_a,
                "bound_b": c.bound_b,
            }
            for c in result.classes
        ],
    }
