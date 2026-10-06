"""Accumulated fatigue damage from counted cycles."""

from __future__ import annotations

from dataclasses import dataclass

from .rainflow import Cycle


@dataclass(frozen=True)
class DamageModel:
    """S-N style damage model.

    Life in cycles: N = (reference_stress / equivalent_amplitude) ** exponent.
    Mean-stress correction is either "none" or "goodman":
    equivalent_amplitude = amplitude / (1 - max(mean, 0) / ultimate_strength).
    """

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
                raise ValueError("goodman requires positive ultimate_strength")

    def equivalent_amplitude(self, cycle: Cycle) -> float | None:
        """Return the equivalent amplitude, or None if not evaluable."""
        if self.mean_correction == "none":
            return cycle.amplitude
        denominator = 1.0 - max(cycle.mean, 0.0) / self.ultimate_strength
        if denominator <= 0.0:
            return None
        return cycle.amplitude / denominator


@dataclass(frozen=True)
class CycleDamage:
    """Damage contribution of one cycle."""

    cycle: Cycle
    evaluable: bool
    equivalent_amplitude: float | None
    life: float | None
    damage: float | None


def assess_cycles(cycles: list[Cycle], model: DamageModel) -> list[CycleDamage]:
    """Evaluate damage per cycle over the whole record (not per segment)."""
    results: list[CycleDamage] = []
    for cycle in cycles:
        if cycle.stress_range == 0.0:
            results.append(CycleDamage(cycle, True, 0.0, None, 0.0))
            continue
        equivalent = model.equivalent_amplitude(cycle)
        if equivalent is None:
            results.append(CycleDamage(cycle, False, None, None, None))
            continue
        life = (model.reference_stress / equivalent) ** model.exponent
        results.append(CycleDamage(cycle, True, equivalent, life, cycle.count / life))
    return results


def total_damage(results: list[CycleDamage]) -> float | None:
    """Total damage, or None when any cycle is not evaluable."""
    total = 0.0
    for result in results:
        if result.damage is None:
            return None
        total += result.damage
    return total
