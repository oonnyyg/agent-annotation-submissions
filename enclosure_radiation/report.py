"""CSV and JSON rendering of solved cases."""

from __future__ import annotations

import csv
import io
import json

from .solver import CaseResult

CSV_HEADER = [
    "case", "surface", "area_m2", "emissivity", "adiabatic",
    "temperature_K", "radiosity_W_m2", "irradiation_W_m2",
    "net_heat_flux_W_m2", "net_power_W", "balance_residual_W",
]


def surfaces_csv(results: list[CaseResult]) -> str:
    buffer = io.StringIO()
    writer = csv.writer(buffer, lineterminator="\n")
    writer.writerow(CSV_HEADER)
    for case in results:
        for surf, residual in zip(case.surfaces, case.residuals):
            writer.writerow([
                case.name, surf.name,
                f"{surf.area:.6g}", f"{surf.emissivity:.6g}",
                "yes" if surf.adiabatic else "no",
                f"{surf.temperature:.6g}", f"{surf.radiosity:.6g}",
                f"{surf.irradiation:.6g}", f"{surf.net_heat_flux:.6g}",
                f"{surf.net_power:.6g}", f"{residual:.6g}",
            ])
    return buffer.getvalue()


def details_json(solved: list[CaseResult], failed: list[dict]) -> str:
    cases = []
    for case in solved:
        names = [s.name for s in case.surfaces]
        pairs = []
        n = len(names)
        for i in range(n):
            for j in range(i + 1, n):
                forward = case.pair_powers[i][j]
                backward = case.pair_powers[j][i]
                pairs.append({
                    "from": names[i],
                    "to": names[j],
                    "power_W": forward,
                    "reverse_power_W": backward,
                    "antisymmetry_residual_W": forward + backward,
                })
        cases.append({
            "name": case.name,
            "status": "solved",
            "surfaces": [
                {
                    "name": s.name,
                    "area_m2": s.area,
                    "emissivity": s.emissivity,
                    "adiabatic": s.adiabatic,
                    "temperature_K": s.temperature,
                    "radiosity_W_m2": s.radiosity,
                    "irradiation_W_m2": s.irradiation,
                    "net_heat_flux_W_m2": s.net_heat_flux,
                    "net_power_W": s.net_power,
                    "balance_residual_W": residual,
                }
                for s, residual in zip(case.surfaces, case.residuals)
            ],
            "pair_exchange_W": pairs,
            "global_energy_residual_W": case.global_residual,
        })
    cases.extend(failed)
    return json.dumps({"cases": cases}, indent=2) + "\n"
