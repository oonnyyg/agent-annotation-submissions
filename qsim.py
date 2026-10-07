"""Small Clifford-circuit state-vector simulator (standard library only)."""

from __future__ import annotations

import math
import random
from dataclasses import dataclass, field

MIN_QUBITS = 1
MAX_QUBITS = 10
TOL = 1e-9

# Single-qubit gates as 2x2 complex matrices, basis (|0>, |1>).
_GATES = {
    "H": [[1 / math.sqrt(2), 1 / math.sqrt(2)], [1 / math.sqrt(2), -1 / math.sqrt(2)]],
    "X": [[0, 1], [1, 0]],
    "S": [[1, 0], [0, 1j]],
}


class CircuitError(ValueError):
    """Raised when a circuit description fails validation."""


@dataclass
class Operation:
    kind: str  # "H" | "X" | "S" | "CX" | "MEASURE"
    qubits: tuple[int, ...]
    classical_bit: int | None = None  # target for MEASURE
    condition: tuple[int, int] | None = None  # (classical_bit, required_value)


@dataclass
class Circuit:
    num_qubits: int
    num_classical_bits: int
    ops: list[Operation] = field(default_factory=list)


def parse_circuit(data: dict) -> Circuit:
    """Validate a JSON-decoded circuit description and build a Circuit."""
    if not isinstance(data, dict):
        raise CircuitError("circuit must be a JSON object")
    nq = data.get("qubits")
    if not isinstance(nq, int) or isinstance(nq, bool) or not (MIN_QUBITS <= nq <= MAX_QUBITS):
        raise CircuitError(f"qubits must be an integer in [{MIN_QUBITS}, {MAX_QUBITS}]")
    nc = data.get("classical_bits", 0)
    if not isinstance(nc, int) or isinstance(nc, bool) or nc < 0:
        raise CircuitError("classical_bits must be a non-negative integer")
    raw_ops = data.get("ops")
    if not isinstance(raw_ops, list):
        raise CircuitError("ops must be a list")

    ops: list[Operation] = []
    for i, raw in enumerate(raw_ops):
        ops.append(_parse_op(i, raw, nq, nc))
    return Circuit(num_qubits=nq, num_classical_bits=nc, ops=ops)


def _check_qubit(i: int, q, nq: int) -> int:
    if not isinstance(q, int) or isinstance(q, bool) or not (0 <= q < nq):
        raise CircuitError(f"op {i}: qubit index {q!r} out of range [0, {nq})")
    return q


def _parse_condition(i: int, raw: dict, nc: int) -> tuple[int, int] | None:
    cond = raw.get("condition")
    if cond is None:
        return None
    if not isinstance(cond, dict):
        raise CircuitError(f"op {i}: condition must be an object")
    bit = cond.get("bit")
    value = cond.get("value")
    if not isinstance(bit, int) or isinstance(bit, bool) or not (0 <= bit < nc):
        raise CircuitError(f"op {i}: condition bit {bit!r} out of range [0, {nc})")
    if value not in (0, 1):
        raise CircuitError(f"op {i}: condition value must be 0 or 1")
    return (bit, value)


def _parse_op(i: int, raw, nq: int, nc: int) -> Operation:
    if not isinstance(raw, dict):
        raise CircuitError(f"op {i}: must be an object")
    gate = raw.get("gate")
    if not isinstance(gate, str):
        raise CircuitError(f"op {i}: missing gate name")
    gate = gate.upper()
    cond = _parse_condition(i, raw, nc)

    if gate in _GATES:
        q = _check_qubit(i, raw.get("qubit"), nq)
        return Operation(kind=gate, qubits=(q,), condition=cond)
    if gate == "CX":
        control = _check_qubit(i, raw.get("control"), nq)
        target = _check_qubit(i, raw.get("target"), nq)
        if control == target:
            raise CircuitError(f"op {i}: CX control and target must differ")
        return Operation(kind="CX", qubits=(control, target), condition=cond)
    if gate == "MEASURE":
        q = _check_qubit(i, raw.get("qubit"), nq)
        bit = raw.get("classical_bit")
        if not isinstance(bit, int) or isinstance(bit, bool) or not (0 <= bit < nc):
            raise CircuitError(f"op {i}: classical_bit {bit!r} out of range [0, {nc})")
        return Operation(kind="MEASURE", qubits=(q,), classical_bit=bit, condition=cond)
    raise CircuitError(f"op {i}: unsupported gate {gate!r}")


@dataclass
class MeasurementRecord:
    op_index: int
    qubit: int
    classical_bit: int
    outcome: int


@dataclass
class ShotResult:
    state: list[complex]  # state vector before the final full measurement
    measurements: list[MeasurementRecord]
    final_bits: str  # bit string, most-significant qubit first
    classical_bits: list[int]


def _apply_single(state: list[complex], matrix: list[list[complex]], qubit: int) -> None:
    step = 1 << qubit
    for base in range(0, len(state), step * 2):
        for off in range(step):
            i0 = base + off
            i1 = i0 + step
            a0, a1 = state[i0], state[i1]
            state[i0] = matrix[0][0] * a0 + matrix[0][1] * a1
            state[i1] = matrix[1][0] * a0 + matrix[1][1] * a1


def _apply_cx(state: list[complex], control: int, target: int) -> None:
    cmask = 1 << control
    tmask = 1 << target
    for i in range(len(state)):
        if (i & cmask) and not (i & tmask):
            j = i | tmask
            state[i], state[j] = state[j], state[i]


def _measure(state: list[complex], qubit: int, rng: random.Random) -> int:
    mask = 1 << qubit
    p1 = sum(abs(a) ** 2 for i, a in enumerate(state) if i & mask)
    p1 = min(max(p1, 0.0), 1.0)
    outcome = 1 if rng.random() < p1 else 0
    norm = math.sqrt(p1 if outcome else 1.0 - p1)
    for i in range(len(state)):
        if bool(i & mask) != bool(outcome):
            state[i] = 0j
        elif norm > 0:
            state[i] /= norm
    return outcome


def _sample_bitstring(state: list[complex], rng: random.Random, num_qubits: int) -> str:
    r = rng.random()
    cumulative = 0.0
    index = len(state) - 1
    for i, a in enumerate(state):
        cumulative += abs(a) ** 2
        if r < cumulative or i == len(state) - 1:
            index = i
            break
    return format(index, f"0{num_qubits}b")


def run_shot(circuit: Circuit, rng: random.Random) -> ShotResult:
    """Run one shot: fresh |0...0> state, fresh zeroed classical bits."""
    state = [0j] * (1 << circuit.num_qubits)
    state[0] = 1.0 + 0j
    cbits = [0] * circuit.num_classical_bits
    records: list[MeasurementRecord] = []

    for idx, op in enumerate(circuit.ops):
        if op.condition is not None:
            bit, value = op.condition
            if cbits[bit] != value:
                continue
        if op.kind == "MEASURE":
            outcome = _measure(state, op.qubits[0], rng)
            cbits[op.classical_bit] = outcome
            records.append(MeasurementRecord(idx, op.qubits[0], op.classical_bit, outcome))
        elif op.kind == "CX":
            _apply_cx(state, op.qubits[0], op.qubits[1])
        else:
            _apply_single(state, _GATES[op.kind], op.qubits[0])

    final = _sample_bitstring(state, rng, circuit.num_qubits)
    return ShotResult(state=state, measurements=records, final_bits=final, classical_bits=list(cbits))


def run_counts(circuit: Circuit, shots: int, seed: int | None = None) -> dict[str, int]:
    """Run multiple shots and tally final bit strings."""
    if shots <= 0:
        raise ValueError("shots must be positive")
    rng = random.Random(seed)
    counts: dict[str, int] = {}
    for _ in range(shots):
        result = run_shot(circuit, rng)
        counts[result.final_bits] = counts.get(result.final_bits, 0) + 1
    return counts
