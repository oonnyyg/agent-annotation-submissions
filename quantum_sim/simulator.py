from __future__ import annotations

import cmath
import math
import random
from dataclasses import dataclass
from typing import Any


class CircuitError(ValueError):
    """Raised when a circuit description is invalid."""


@dataclass(frozen=True)
class Measurement:
    operation: int
    qubit: int
    classical_bit: int
    outcome: int
    probability: float


@dataclass(frozen=True)
class Sample:
    counts: dict[str, int]
    frequencies: dict[str, float]


class Simulator:
    def __init__(self, num_qubits: int, num_classical_bits: int = 0, *, tolerance: float = 1e-10):
        if not isinstance(num_qubits, int) or isinstance(num_qubits, bool):
            raise CircuitError("num_qubits must be an integer")
        if not 1 <= num_qubits <= 10:
            raise CircuitError("num_qubits must be between 1 and 10")
        if not isinstance(num_classical_bits, int) or isinstance(num_classical_bits, bool):
            raise CircuitError("num_classical_bits must be an integer")
        if num_classical_bits < 0:
            raise CircuitError("num_classical_bits must be non-negative")
        self.num_qubits = num_qubits
        self.num_classical_bits = num_classical_bits
        self.tolerance = tolerance
        self._dimension = 1 << num_qubits

    def initial_state(self) -> list[complex]:
        state = [0j] * self._dimension
        state[0] = 1 + 0j
        return state

    def initial_classical_bits(self) -> list[int]:
        return [0] * self.num_classical_bits

    def apply_gate(self, state: list[complex], gate: str, qubit: int) -> None:
        self._validate_qubit(qubit)
        bit = 1 << qubit
        if gate == "H":
            factor = 1 / math.sqrt(2)
            for index in range(self._dimension):
                if index & bit == 0:
                    other = index | bit
                    zero_amplitude = state[index]
                    one_amplitude = state[other]
                    state[index] = factor * (zero_amplitude + one_amplitude)
                    state[other] = factor * (zero_amplitude - one_amplitude)
        elif gate == "X":
            for index in range(self._dimension):
                if index & bit == 0:
                    other = index | bit
                    state[index], state[other] = state[other], state[index]
        elif gate == "S":
            for index in range(self._dimension):
                if index & bit:
                    state[index] *= 1j
        else:
            raise CircuitError(f"unknown single-qubit gate: {gate!r}")

    def apply_cnot(self, state: list[complex], control: int, target: int) -> None:
        self._validate_qubit(control, "control")
        self._validate_qubit(target, "target")
        if control == target:
            raise CircuitError("CNOT control and target must be different qubits")
        control_bit = 1 << control
        target_bit = 1 << target
        for index in range(self._dimension):
            if index & control_bit and not index & target_bit:
                other = index | target_bit
                state[index], state[other] = state[other], state[index]

    def measure(
        self,
        state: list[complex],
        qubit: int,
        classical_bit: int,
        rng: random.Random,
    ) -> Measurement:
        self._validate_qubit(qubit)
        self._validate_classical_bit(classical_bit)
        bit = 1 << qubit
        probability = sum(
            abs(amplitude) ** 2
            for index, amplitude in enumerate(state)
            if index & bit
        )
        outcome = 1 if rng.random() < probability else 0
        norm = math.sqrt(probability if outcome else 1.0 - probability)
        if norm <= self.tolerance:
            raise CircuitError("cannot collapse a state with probability below tolerance")
        scale = 1.0 / norm
        for index in range(self._dimension):
            if bool(index & bit) != bool(outcome):
                state[index] = 0j
            else:
                state[index] *= scale
        self.assert_normalized(state)
        return Measurement(-1, qubit, classical_bit, outcome, probability)

    def final_measurement(self, state: list[complex], rng: random.Random) -> tuple[str, Measurement]:
        probabilities = [abs(amplitude) ** 2 for amplitude in state]
        index = rng.choices(range(self._dimension), weights=probabilities, k=1)[0]
        bits = "".join("1" if index & (1 << qubit) else "0" for qubit in reversed(range(self.num_qubits)))
        collapsed = [0j] * self._dimension
        collapsed[index] = state[index] / cmath.sqrt(probabilities[index]) if probabilities[index] > self.tolerance else 1 + 0j
        probability = probabilities[index]
        return bits, Measurement(-1, -1, -1, index, probability), collapsed

    def assert_normalized(self, state: list[complex]) -> None:
        if len(state) != self._dimension:
            raise CircuitError("state vector has an invalid length")
        norm = sum(abs(amplitude) ** 2 for amplitude in state)
        if not math.isclose(norm, 1.0, abs_tol=self.tolerance, rel_tol=0.0):
            raise CircuitError(f"state is not normalized: norm squared={norm!r}")

    def _validate_qubit(self, qubit: Any, name: str = "qubit") -> None:
        if not isinstance(qubit, int) or isinstance(qubit, bool):
            raise CircuitError(f"{name} must be an integer")
        if not 0 <= qubit < self.num_qubits:
            raise CircuitError(f"{name} {qubit} is outside 0..{self.num_qubits - 1}")

    def _validate_classical_bit(self, bit: Any) -> None:
        if not isinstance(bit, int) or isinstance(bit, bool):
            raise CircuitError("classical bit must be an integer")
        if not 0 <= bit < self.num_classical_bits:
            raise CircuitError(f"classical bit {bit} is outside 0..{self.num_classical_bits - 1}")
