"""Ising lattice definition, validation, and energy helpers."""

from __future__ import annotations

import math
from dataclasses import dataclass, field


@dataclass(frozen=True)
class IsingConfig:
    """Physical parameters of one simulation run."""

    size: int
    coupling: float
    temperature: float
    field: float = 0.0
    initial_spins: tuple[int, ...] | None = None

    def __post_init__(self) -> None:
        if not isinstance(self.size, int) or isinstance(self.size, bool):
            raise ValueError("size must be an integer")
        if not 3 <= self.size <= 64:
            raise ValueError("size must be between 3 and 64")
        for name, value in (
            ("coupling", self.coupling),
            ("temperature", self.temperature),
            ("field", self.field),
        ):
            if not isinstance(value, (int, float)) or isinstance(value, bool):
                raise ValueError(f"{name} must be a real number")
            if not math.isfinite(value):
                raise ValueError(f"{name} must be finite")
        if self.coupling <= 0.0:
            raise ValueError("coupling must be positive")
        if self.temperature <= 0.0:
            raise ValueError("temperature must be positive")
        if self.initial_spins is not None:
            spins = tuple(self.initial_spins)
            if len(spins) != self.size * self.size:
                raise ValueError("initial_spins must contain size*size entries")
            if any(s not in (1, -1) for s in spins):
                raise ValueError("initial spins must be +1 or -1")
            object.__setattr__(self, "initial_spins", spins)

    def starting_spins(self) -> list[int]:
        if self.initial_spins is not None:
            return list(self.initial_spins)
        return [1] * (self.size * self.size)


@dataclass
class IsingLattice:
    """Square lattice with periodic boundaries, spins stored row-major."""

    config: IsingConfig
    spins: list[int] = field(init=False)

    def __post_init__(self) -> None:
        self.spins = self.config.starting_spins()

    @property
    def size(self) -> int:
        return self.config.size

    def neighbor_sum(self, index: int) -> int:
        n = self.size
        row, col = divmod(index, n)
        s = self.spins
        return (
            s[row * n + (col + 1) % n]
            + s[row * n + (col - 1) % n]
            + s[((row + 1) % n) * n + col]
            + s[((row - 1) % n) * n + col]
        )

    def total_energy(self, spins: list[int] | None = None) -> float:
        """Each nearest-neighbor bond counted once, minus field * magnetization."""
        s = spins if spins is not None else self.spins
        n = self.size
        j = self.config.coupling
        h = self.config.field
        bond_sum = 0
        magnetization = 0
        for row in range(n):
            for col in range(n):
                spin = s[row * n + col]
                bond_sum += spin * s[row * n + (col + 1) % n]
                bond_sum += spin * s[((row + 1) % n) * n + col]
                magnetization += spin
        return -j * bond_sum - h * magnetization

    def total_magnetization(self) -> int:
        return sum(self.spins)

    def flip_delta_energy(self, index: int) -> float:
        """Energy change if the spin at index is flipped."""
        spin = self.spins[index]
        return 2.0 * spin * (
            self.config.coupling * self.neighbor_sum(index) + self.config.field
        )

    def flip(self, index: int) -> None:
        self.spins[index] = -self.spins[index]
