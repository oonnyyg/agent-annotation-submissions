"""单点 Metropolis 采样：热化与采样分离，逐温度独立随机序列。"""
from __future__ import annotations

import math
import random
from dataclasses import dataclass

from .config import ExperimentConfig, TemperatureConfig
from .model import Lattice


@dataclass(frozen=True)
class SampleRecord:
    sweep: int
    energy: float
    magnetization: int


@dataclass(frozen=True)
class TemperatureResult:
    temperature: float
    samples: tuple[SampleRecord, ...]
    energy_per_spin: float
    abs_magnetization_per_spin: float
    specific_heat_per_spin: float
    susceptibility_per_spin: float


def _rng_for(seed: int, temperature: float) -> random.Random:
    """每个温度独立的随机序列：仅依赖 (seed, temperature)，与温度顺序无关。"""
    return random.Random(f"ising|{seed}|{temperature!r}")


def _sweep(lattice: Lattice, rng: random.Random, beta: float) -> None:
    """一轮扫描：n_spins 次随机选点（允许重复）的 Metropolis 翻转尝试。"""
    randrange = rng.randrange
    random_float = rng.random
    n = lattice.n_spins
    for _ in range(n):
        index = randrange(n)
        delta = lattice.flip_delta_energy(index)
        if delta <= 0.0 or random_float() < math.exp(-beta * delta):
            lattice.flip(index)


def _population_variance(values: list[float], mean: float) -> float:
    return sum((v - mean) ** 2 for v in values) / len(values)


def run_temperature(config: ExperimentConfig, temp_config: TemperatureConfig) -> TemperatureResult:
    """从相同初态独立运行一个温度点，返回样本与统计量。"""
    lattice = Lattice(config.size, config.coupling, config.field, config.initial_spins)
    rng = _rng_for(config.seed, temp_config.temperature)
    beta = 1.0 / temp_config.temperature
    sweep_count = 0
    for _ in range(temp_config.thermalization_sweeps):
        _sweep(lattice, rng, beta)
        sweep_count += 1
    samples: list[SampleRecord] = []
    for _ in range(temp_config.n_samples):
        for _ in range(temp_config.sample_every):
            _sweep(lattice, rng, beta)
            sweep_count += 1
        samples.append(
            SampleRecord(
                sweep=sweep_count,
                energy=lattice.total_energy(),
                magnetization=lattice.total_magnetization(),
            )
        )
    n_spins = config.n_spins
    n = len(samples)
    energies = [s.energy for s in samples]
    mags = [float(s.magnetization) for s in samples]
    mean_e = sum(energies) / n
    mean_abs_m = sum(abs(m) for m in mags) / n
    var_e = _population_variance(energies, mean_e)
    mean_m = sum(mags) / n
    var_m = _population_variance(mags, mean_m)
    temperature = temp_config.temperature
    return TemperatureResult(
        temperature=temperature,
        samples=tuple(samples),
        energy_per_spin=mean_e / n_spins,
        abs_magnetization_per_spin=mean_abs_m / n_spins,
        specific_heat_per_spin=var_e / (n_spins * temperature * temperature),
        susceptibility_per_spin=var_m / (n_spins * temperature),
    )
