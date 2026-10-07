from __future__ import annotations

import math
import random
from dataclasses import dataclass
from typing import Any

from .simulator import CircuitError, Sample, Simulator


@dataclass(frozen=True)
class DiagnosticResult:
    measurements: list[dict[str, float | int]]
    state_vector: list[dict[str, float]]
    state_vector_norm_squared: float
    final_bitstring: str


def load_circuit(data: Any) -> tuple[int, int | None, list[dict[str, Any]]]:
    if not isinstance(data, dict):
        raise CircuitError("circuit must be a JSON object")
    num_qubits = data.get("num_qubits")
    if not isinstance(num_qubits, int) or isinstance(num_qubits, bool) or not 1 <= num_qubits <= 10:
        raise CircuitError("num_qubits must be an integer between 1 and 10")
    num_classical_bits = data.get("num_classical_bits")
    if num_classical_bits is not None and (
        not isinstance(num_classical_bits, int)
        or isinstance(num_classical_bits, bool)
        or num_classical_bits < 0
    ):
        raise CircuitError("num_classical_bits must be a non-negative integer or null")
    operations = data.get("operations", [])
    if not isinstance(operations, list):
        raise CircuitError("operations must be a list")
    return num_qubits, num_classical_bits, operations


def required_classical_bits(operations: list[dict[str, Any]]) -> int:
    highest = -1
    for position, operation in enumerate(operations):
        if not isinstance(operation, dict):
            raise CircuitError(f"operation {position} must be an object")
        condition = operation.get("condition")
        if condition is not None:
            if not isinstance(condition, dict):
                raise CircuitError(f"operation {position} condition must be an object")
            bit = condition.get("classical_bit")
            if _is_index(bit):
                highest = max(highest, bit)
            expected = condition.get("equals")
            if expected not in (0, 1):
                raise CircuitError(f"operation {position} condition.equals must be 0 or 1")
        if operation.get("gate") == "measure":
            classical_bit = operation.get("classical_bit")
            if _is_index(classical_bit):
                highest = max(highest, classical_bit)
    return highest + 1


def validate_circuit(num_qubits: int, num_classical_bits: int, operations: list[dict[str, Any]]) -> None:
    simulator = Simulator(num_qubits, num_classical_bits)
    for position, operation in enumerate(operations):
        gate = operation.get("gate")
        if gate in ("H", "X", "S"):
            simulator._validate_qubit(operation.get("qubit"), "qubit")
        elif gate == "CNOT":
            simulator._validate_qubit(operation.get("control"), "control")
            simulator._validate_qubit(operation.get("target"), "target")
            if operation["control"] == operation["target"]:
                raise CircuitError(f"operation {position}: CNOT control and target must differ")
        elif gate == "measure":
            simulator._validate_qubit(operation.get("qubit"), "qubit")
            simulator._validate_classical_bit(operation.get("classical_bit"))
        else:
            raise CircuitError(f"operation {position}: unsupported gate {gate!r}")
        _validate_condition(operation, num_classical_bits, position)
        unknown = set(operation) - {"gate", "qubit", "control", "target", "classical_bit", "condition"}
        if unknown:
            raise CircuitError(f"operation {position}: unknown fields {sorted(unknown)}")


def run_diagnostic(data: dict[str, Any], seed: int | None = None) -> DiagnosticResult:
    num_qubits, explicit_classical, operations = load_circuit(data)
    num_classical_bits = explicit_classical if explicit_classical is not None else required_classical_bits(operations)
    validate_circuit(num_qubits, num_classical_bits, operations)
    simulator = Simulator(num_qubits, num_classical_bits)
    state = simulator.initial_state()
    classical = simulator.initial_classical_bits()
    rng = random.Random(seed)
    records: list[dict[str, float | int]] = []
    for position, operation in enumerate(operations):
        if not _condition_matches(operation, classical):
            continue
        gate = operation["gate"]
        if gate in ("H", "X", "S"):
            simulator.apply_gate(state, gate, operation["qubit"])
        elif gate == "CNOT":
            simulator.apply_cnot(state, operation["control"], operation["target"])
        else:
            record = simulator.measure(state, operation["qubit"], operation["classical_bit"], rng)
            classical[record.classical_bit] = record.outcome
            records.append(
                {
                    "operation": position,
                    "qubit": record.qubit,
                    "classical_bit": record.classical_bit,
                    "outcome": record.outcome,
                    "probability": record.probability,
                }
            )
        simulator.assert_normalized(state)
    final_bits, final_record, _ = simulator.final_measurement(state, rng)
    return DiagnosticResult(
        measurements=records,
        state_vector=[_complex_to_json(value) for value in state],
        state_vector_norm_squared=sum(abs(value) ** 2 for value in state),
        final_bitstring=final_bits,
    )


def sample_circuit(data: dict[str, Any], shots: int, seed: int | None = None) -> Sample:
    if not isinstance(shots, int) or isinstance(shots, bool) or shots < 1:
        raise CircuitError("shots must be a positive integer")
    num_qubits, explicit_classical, operations = load_circuit(data)
    num_classical_bits = explicit_classical if explicit_classical is not None else required_classical_bits(operations)
    validate_circuit(num_qubits, num_classical_bits, operations)
    simulator = Simulator(num_qubits, num_classical_bits)
    rng = random.Random(seed)
    counts = {format(index, f"0{num_qubits}b"): 0 for index in range(1 << num_qubits)}
    for _ in range(shots):
        state = simulator.initial_state()
        classical = simulator.initial_classical_bits()
        for operation in operations:
            if not _condition_matches(operation, classical):
                continue
            gate = operation["gate"]
            if gate in ("H", "X", "S"):
                simulator.apply_gate(state, gate, operation["qubit"])
            elif gate == "CNOT":
                simulator.apply_cnot(state, operation["control"], operation["target"])
            else:
                record = simulator.measure(state, operation["qubit"], operation["classical_bit"], rng)
                classical[record.classical_bit] = record.outcome
        bits, _, _ = simulator.final_measurement(state, rng)
        counts[bits] += 1
    frequencies = {bits: count / shots for bits, count in counts.items()}
    return Sample(counts, frequencies)


def _condition_matches(operation: dict[str, Any], classical: list[int]) -> bool:
    condition = operation.get("condition")
    if condition is None:
        return True
    return classical[condition["classical_bit"]] == condition["equals"]


def _validate_condition(operation: dict[str, Any], num_classical_bits: int, position: int) -> None:
    condition = operation.get("condition")
    if condition is None:
        return
    bit = condition.get("classical_bit")
    if not _is_index(bit) or bit >= num_classical_bits:
        raise CircuitError(f"operation {position}: condition classical bit is invalid")


def _is_index(value: Any) -> bool:
    return isinstance(value, int) and not isinstance(value, bool) and value >= 0


def _complex_to_json(value: complex) -> dict[str, float]:
    if math.isclose(value.imag, 0.0, abs_tol=1e-15):
        return {"real": value.real, "imag": 0.0}
    return {"real": value.real, "imag": value.imag}
