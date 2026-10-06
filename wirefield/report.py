"""Scenario evaluation, safety-distance gating, and report building."""

from __future__ import annotations

from .coil import Coil
from .field import coil_min_distance, field_per_coil, total_field
from .probe import Probe
from .vector import Vec3, norm, sub


def _vec_record(v: Vec3) -> dict:
    return {"x": v[0], "y": v[1], "z": v[2], "magnitude": norm(v)}


def _diff_record(a: Vec3, b: Vec3) -> dict:
    """Vector difference a-b; deliberately NOT a difference of magnitudes."""
    return _vec_record(sub(a, b))


def _vec(rec: dict) -> Vec3:
    return (rec["x"], rec["y"], rec["z"])


def evaluate_point(
    coils: list[Coil],
    probe: Probe,
    scenarios: list[dict],
    safe_distance: float,
) -> dict:
    """Evaluate one probe point for the baseline and all scenarios."""
    record: dict = {
        "point": {"x": probe.point[0], "y": probe.point[1], "z": probe.point[2]},
        "computable": True,
        "blocked_by": [],
    }
    too_close = [
        coil.coil_id
        for coil in coils
        if coil_min_distance(coil, probe.point) <= safe_distance
    ]
    if too_close:
        record["computable"] = False
        record["blocked_by"] = too_close
        record["reason"] = (
            "point within safe distance of coil(s): " + ", ".join(too_close)
        )
        return record

    def scenario_block(currents: dict[str, float] | None) -> dict:
        per_coil = field_per_coil(coils, probe.point, currents)
        total = total_field(per_coil)
        return {
            "per_coil": {cid: _vec_record(v) for cid, v in per_coil.items()},
            "total_world": _vec_record(total),
            "total_local": _vec_record(probe.local_components(total)),
        }

    baseline = scenario_block(None)
    record["baseline"] = baseline
    record["scenarios"] = []
    for sc in scenarios:
        block = scenario_block(sc["currents"])
        record["scenarios"].append(
            {
                "name": sc["name"],
                "currents": sc["currents"],
                **block,
                "diff_world_vs_baseline": _diff_record(
                    _vec(block["total_world"]), _vec(baseline["total_world"])
                ),
                "diff_local_vs_baseline": _diff_record(
                    _vec(block["total_local"]), _vec(baseline["total_local"])
                ),
            }
        )
    return record


def build_report(
    coils: list[Coil],
    probes: list[Probe],
    scenarios: list[dict],
    safe_distance: float,
) -> dict:
    return {
        "safe_distance": safe_distance,
        "coils": [
            {"id": c.coil_id, "current": c.current, "vertices": len(c.vertices)}
            for c in coils
        ],
        "points": [
            evaluate_point(coils, probe, scenarios, safe_distance) for probe in probes
        ],
    }
