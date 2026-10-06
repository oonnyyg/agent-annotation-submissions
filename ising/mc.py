"""Single-spin Metropolis sampling."""

from __future__ import annotations

import math
import random
from dataclasses import dataclass

from .model import IsingConfig, IsingLattice


@dataclass(frozen=True)
class RunSpec:
    """Monte Carlo schedule shared by every temperature."""

    equilibration_sweeps: int
    sampling_sweeps: int
    sample_interval: int
    seed: int

    def __post_init__(self) -> None:
        for name in ("equilibration_sweeps", "sampling_sweeps", "sample_interval"):
            value = getattr(self, name)
            if not isinstance(value, int) or isinstance(value, bool) or value <= 0:
                raise ValueError(f"{name} must be a positive integer")
        if not isinstance(self.seed, int) or isinstance(self.seed, bool):
            raise ValueError("seed must be an integer")


@dataclass(frozen=True)
class Sample:
    sweep: int
    energy: float
    magnetization: int


def _sweep(lattice: IsingLattice, rng: random.Random, beta: float) -> None:
    n_sites = lattice.size * lattice.size
    for _ in range(n_sites):
        index = rng.randrange(n_sites)
        delta = lattice.flip_delta_energy(index)
        if delta <= 0.0 or rng.random() < math.exp(-beta * delta):
            lattice.flip(index)


def run_temperature(config: IsingConfig, spec: RunSpec, temperature_index: int) -> list[Sample]:
    """Run one temperature independently.

    The RNG is seeded from (seed, temperature_index) so reordering the
    temperature list never changes a given temperature's random stream.
    """
    rng = random.Random(f"ising:{spec.seed}:{temperature_index}")
    lattice = IsingLattice(config)
    beta = 1.0 / config.temperature
    for _ in range(spec.equilibration_sweeps):
        _sweep(lattice, rng, beta)
    samples: list[Sample] = []
    for sweep in range(1, spec.sampling_sweeps + 1):
        _sweep(lattice, rng, beta)
        if sweep % spec.sample_interval == 0:
            samples.append(
                Sample(
                    sweep=sweep,
                    energy=lattice.total_energy(),
                    magnetization=lattice.total_magnetization(),
                )
            )
    return samples
