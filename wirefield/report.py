"""Assemble JSON/CSV reports for baseline and alternative current schemes."""

from __future__ import annotations

from .model import Coil
from .probe import Probe
from .scenario import Scenario
from .solver import PointResult, evaluate
from .vector import Vec3, norm, sub


def _vec(v: Vec3) -> list[float]:
    return [v[0], v[1], v[2]]


def _point_entry(
    coils: list[Coil],
    probe: Probe,
    result: PointResult,
    baseline: PointResult | None,
) -> dict:
    entry: dict = {
        "probe": probe.name,
        "position_m": _vec(probe.position),
        "computable": result.computable,
    }
    if not result.computable:
        entry["blocking_coils"] = list(result.blocking_coils)
        return entry
    total_local = probe.to_local(result.total)
    entry["per_coil_world_T"] = {
        coil.name: _vec(field) for coil, field in zip(coils, result.per_coil)
    }
    entry["total_world_T"] = _vec(result.total)
    entry["total_local_T"] = _vec(total_local)
    entry["magnitude_T"] = norm(result.total)
    if baseline is not None and baseline.computable:
        d_world = sub(result.total, baseline.total)
        d_local = sub(total_local, probe.to_local(baseline.total))
        entry["delta_vs_baseline"] = {
            "world_T": _vec(d_world),
            "local_T": _vec(d_local),
            "world_delta_magnitude_T": norm(d_world),
        }
    return entry


def build_report(
    coils: list[Coil],
    probes: list[Probe],
    scenarios: list[Scenario],
    safe_distance: float,
) -> dict:
    """Evaluate every probe under every current scheme.

    scenarios[0] is the baseline; later entries are compared against it
    as vector differences (never magnitude differences).
    """
    baseline = scenarios[0]
    baseline_results = [
        evaluate(coils, list(baseline.currents), p.position, safe_distance) for p in probes
    ]
    scheme_entries = []
    for scenario in scenarios:
        if scenario is baseline:
            results = baseline_results
        else:
            results = [
                evaluate(coils, list(scenario.currents), p.position, safe_distance)
                for p in probes
            ]
        points = []
        for probe, result, base in zip(probes, results, baseline_results):
            points.append(_point_entry(coils, probe, result, None if scenario is baseline else base))
        scheme_entries.append(
            {
                "name": scenario.name,
                "currents_A": {c.name: i for c, i in zip(coils, scenario.currents)},
                "points": points,
            }
        )
    return {
        "mu0": "4*pi*1e-7 H/m",
        "safe_distance_m": safe_distance,
        "coils": [c.name for c in coils],
        "scenarios": scheme_entries,
    }
