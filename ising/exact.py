"""Exact enumeration over all spin states for small lattices (L = 3 or 4)."""

from __future__ import annotations

import math

from .model import IsingConfig, IsingLattice


def exact_observables(config: IsingConfig) -> dict[str, float | int]:
    """Exact canonical averages with numerically stable Boltzmann weights.

    Weights are computed as exp(-beta * (E - E_min)) so no overflow occurs.
    Uses the same population-moment definitions as the sampling summary.
    """
    if config.size not in (3, 4):
        raise ValueError("exact enumeration is only supported for size 3 or 4")
    lattice = IsingLattice(config)
    n_sites = config.size * config.size
    beta = 1.0 / config.temperature

    energies: list[float] = []
    magnetizations: list[int] = []
    for state in range(1 << n_sites):
        spins = [1 if (state >> i) & 1 else -1 for i in range(n_sites)]
        energies.append(lattice.total_energy(spins))
        magnetizations.append(sum(spins))

    e_min = min(energies)
    weights = [math.exp(-beta * (e - e_min)) for e in energies]
    z = sum(weights)

    mean_e = sum(w * e for w, e in zip(weights, energies)) / z
    mean_m = sum(w * m for w, m in zip(weights, magnetizations)) / z
    mean_abs_m = sum(w * abs(m) for w, m in zip(weights, magnetizations)) / z
    var_e = sum(w * (e - mean_e) ** 2 for w, e in zip(weights, energies)) / z
    var_m = sum(w * (m - mean_m) ** 2 for w, m in zip(weights, magnetizations)) / z

    return {
        "n_states": 1 << n_sites,
        "energy_per_spin": mean_e / n_sites,
        "abs_magnetization_per_spin": mean_abs_m / n_sites,
        "magnetization_per_spin": mean_m / n_sites,
        "energy_variance": var_e,
        "magnetization_variance": var_m,
        "heat_capacity_per_spin": var_e / (n_sites * config.temperature**2),
        "susceptibility_per_spin": var_m / (n_sites * config.temperature),
    }
