"""小晶格（L=3 或 4）全状态枚举的精确均值与响应量。"""
from __future__ import annotations

import math
from dataclasses import dataclass

from .model import Lattice


@dataclass(frozen=True)
class ExactObservables:
    temperature: float
    n_states: int
    energy_per_spin: float
    abs_magnetization_per_spin: float
    specific_heat_per_spin: float
    susceptibility_per_spin: float


def _state_energy_magnetization(size: int, coupling: float, field: float, bits: int) -> tuple[float, int]:
    """从位图构造自旋并计算总能量与总磁化（位 1 -> +1，位 0 -> -1）。"""
    spins = tuple(1 if (bits >> i) & 1 else -1 for i in range(size * size))
    lattice = Lattice(size, coupling, field, spins)
    return lattice.total_energy(), lattice.total_magnetization()


def exact_observables(size: int, coupling: float, field: float, temperature: float) -> ExactObservables:
    """权重用平移能量 w = exp(-beta*(E - E_min)) 计算，数值稳定。"""
    if size not in (3, 4):
        raise ValueError("精确枚举仅支持边长 3 或 4")
    n_spins = size * size
    n_states = 1 << n_spins
    beta = 1.0 / temperature
    energies = [0.0] * n_states
    magnetizations = [0] * n_states
    e_min = float("inf")
    for bits in range(n_states):
        energy, mag = _state_energy_magnetization(size, coupling, field, bits)
        energies[bits] = energy
        magnetizations[bits] = mag
        if energy < e_min:
            e_min = energy
    weights = [math.exp(-beta * (e - e_min)) for e in energies]
    z = sum(weights)
    mean_e = sum(w * e for w, e in zip(weights, energies)) / z
    mean_e2 = sum(w * e * e for w, e in zip(weights, energies)) / z
    mean_abs_m = sum(w * abs(m) for w, m in zip(weights, magnetizations)) / z
    mean_m = sum(w * m for w, m in zip(weights, magnetizations)) / z
    mean_m2 = sum(w * m * m for w, m in zip(weights, magnetizations)) / z
    var_e = mean_e2 - mean_e * mean_e
    var_m = mean_m2 - mean_m * mean_m
    return ExactObservables(
        temperature=temperature,
        n_states=n_states,
        energy_per_spin=mean_e / n_spins,
        abs_magnetization_per_spin=mean_abs_m / n_spins,
        specific_heat_per_spin=var_e / (n_spins * temperature * temperature),
        susceptibility_per_spin=var_m / (n_spins * temperature),
    )
