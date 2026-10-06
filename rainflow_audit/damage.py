"""Fatigue damage accumulation and JSON summary."""
from __future__ import annotations

import json
from dataclasses import dataclass

from .rainflow import Cycle


@dataclass(frozen=True)
class SNModel:
    """S-N model: life = (reference_stress / equivalent_amplitude) ** exponent."""

    reference_stress: float
    exponent: float
    mean_correction: str = "none"
    ultimate_strength: float | None = None

    def __post_init__(self) -> None:
        if self.reference_stress <= 0:
            raise ValueError("reference_stress must be positive")
        if self.exponent <= 0:
            raise ValueError("exponent must be positive")
        if self.mean_correction not in ("none", "goodman"):
            raise ValueError("mean_correction must be 'none' or 'goodman'")
        if self.mean_correction == "goodman":
            if self.ultimate_strength is None or self.ultimate_strength <= 0:
                raise ValueError("goodman correction requires positive ultimate_strength")

    @classmethod
    def from_json(cls, path: str) -> "SNModel":
        with open(path, encoding="utf-8-sig") as handle:
            data = json.load(handle)
        return cls(
            reference_stress=float(data["reference_stress"]),
            exponent=float(data["exponent"]),
            mean_correction=str(data.get("mean_correction", "none")),
            ultimate_strength=(
                None
                if data.get("ultimate_strength") is None
                else float(data["ultimate_strength"])
            ),
        )


@dataclass(frozen=True)
class CycleDamage:
    cycle: Cycle
    equivalent_amplitude: float | None
    life: float | None
    damage: float
    evaluable: bool
    note: str | None


def evaluate_cycles(cycles: list[Cycle], model: SNModel) -> list[CycleDamage]:
    return [_evaluate_cycle(cycle, model) for cycle in cycles]


def _evaluate_cycle(cycle: Cycle, model: SNModel) -> CycleDamage:
    if cycle.stress_range == 0.0:
        return CycleDamage(cycle, None, None, 0.0, False, "zero range cycle")
    amplitude = cycle.amplitude
    if model.mean_correction == "goodman":
        denominator = 1.0 - max(cycle.mean, 0.0) / model.ultimate_strength
        if denominator <= 0.0:
            return CycleDamage(
                cycle, None, None, 0.0, False,
                "goodman denominator non-positive; cycle not evaluable",
            )
        equivalent = amplitude / denominator
    else:
        equivalent = amplitude
    life = (model.reference_stress / equivalent) ** model.exponent
    return CycleDamage(cycle, equivalent, life, cycle.count / life, True, None)


def summarize(results: list[CycleDamage]) -> dict:
    """Build the JSON summary over the whole record (no per-segment sums)."""
    inevaluable = [r for r in results if not r.evaluable and r.cycle.stress_range > 0]
    return {
        "cycle_count": sum(r.cycle.count for r in results),
        "cycles_recorded": len(results),
        "cycles_evaluable": sum(1 for r in results if r.evaluable),
        "cycles_inevaluable": len(inevaluable),
        "total_damage": sum(r.damage for r in results),
        "damage_complete": not inevaluable,
        "warning": (
            "some cycles were not evaluable; total damage is a lower bound"
            if inevaluable
            else None
        ),
        "cycles": [
            {
                "start_time": r.cycle.start_time,
                "end_time": r.cycle.end_time,
                "stress_range": r.cycle.stress_range,
                "mean": r.cycle.mean,
                "amplitude": r.cycle.amplitude,
                "count": r.cycle.count,
                "equivalent_amplitude": r.equivalent_amplitude,
                "life": r.life,
                "damage": r.damage,
                "evaluable": r.evaluable,
                "note": r.note,
            }
            for r in results
        ],
    }
