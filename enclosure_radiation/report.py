"""CSV and JSON reporting for solved cases."""

from __future__ import annotations

import csv
import json
import os

from .model import Enclosure
from .solver import CaseResult

CSV_FIELDS = [
    "case", "surface", "temperature_K", "radiosity_W_m2",
    "irradiation_W_m2", "net_heat_flux_W_m2", "net_power_W",
]


def write_surfaces_csv(results: list[CaseResult], path: str) -> None:
    _ensure_parent(path)
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(CSV_FIELDS)
        for res in results:
            for s in res.surfaces:
                writer.writerow([
                    res.name, s.name,
                    f"{s.temperature_K:.10g}", f"{s.radiosity:.10g}",
                    f"{s.irradiation:.10g}", f"{s.net_heat_flux:.10g}",
                    f"{s.net_power:.10g}",
                ])


def build_exchange_json(enc: Enclosure, results: list[CaseResult]) -> dict:
    names = [s.name for s in enc.surfaces]
    cases = []
    for res in results:
        pairs = []
        max_asym = 0.0
        n = len(names)
        for i in range(n):
            for j in range(i + 1, n):
                pij = res.exchange[i][j]
                pji = res.exchange[j][i]
                max_asym = max(max_asym, abs(pij + pji))
                if pij != 0.0 or pji != 0.0:
                    pairs.append({
                        "from": names[i], "to": names[j],
                        "power_W": pij,
                    })
        per_surface = [
            {"surface": s.name, "net_power_W": s.net_power}
            for s in res.surfaces
        ]
        cases.append({
            "case": res.name,
            "pairwise_exchange_W": pairs,
            "antisymmetry_max_residual_W": max_asym,
            "per_surface_balance_W": per_surface,
            "enclosure_energy_residual_W": res.energy_residual,
        })
    return {"cases": cases}


def write_exchange_json(enc: Enclosure, results: list[CaseResult],
                        path: str) -> None:
    _ensure_parent(path)
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(build_exchange_json(enc, results), fh, indent=2)
        fh.write("\n")


def _ensure_parent(path: str) -> None:
    parent = os.path.dirname(os.path.abspath(path))
    os.makedirs(parent, exist_ok=True)
