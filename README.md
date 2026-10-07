# Clifford Shot Simulator

A small quantum-circuit simulator using only the Python standard library.
It evolves a local complex state vector, supports mid-circuit measurement
with classical feedback, and never talks to quantum hardware.

## Usage

Single-shot diagnostic (mid-circuit measurement log + state vector before
the final full measurement):

```
python cli.py examples/feedback.json --seed 42
```

Multi-shot sampling (counts and frequencies of final bit strings):

```
python cli.py examples/bell.json --shots 1000 --seed 7
```

`--seed` makes any run exactly reproducible. Omit `--shots` for diagnostic
mode. Each shot re-initializes the quantum state to `|0...0>` and all
classical bits to `0`, so earlier shots never contaminate later ones.

## Circuit JSON format

```json
{
  "qubits": 2,
  "classical_bits": 1,
  "ops": [
    {"gate": "H", "qubit": 0},
    {"gate": "MEASURE", "qubit": 0, "classical_bit": 0},
    {"gate": "X", "qubit": 1, "condition": {"bit": 0, "value": 1}}
  ]
}
```

- `qubits`: integer 1–10. `classical_bits`: non-negative integer (default 0).
- Gates: `H`, `X`, `S` (take `qubit`), `CX` (takes `control` and `target`,
  which must differ), `MEASURE` (takes `qubit` and `classical_bit`).
- Any op may carry `"condition": {"bit": c, "value": 0|1}`; it executes only
  when classical bit `c` currently equals `value`, otherwise the quantum
  state is left untouched.
- The whole circuit (all qubit/classical indices, gate names, conditions)
  is validated before anything runs; invalid input exits with an error.

## Bit ordering

Qubit 0 is the least-significant bit of the state index. Printed bit strings
are ordered most-significant qubit first: for 2 qubits, `"10"` means
qubit 1 = 1, qubit 0 = 0 (state index 2).

## Semantics

- Mid-circuit `MEASURE` samples the outcome from the Born probabilities,
  collapses and renormalizes the state vector, and writes the outcome to the
  target classical bit.
- Superpositions are true complex amplitudes, never replaced by random
  booleans; phases (e.g. from `S`) interfere correctly in later gates.
- At the end of each shot every qubit is measured in the computational
  basis by sampling from the final state vector.

## Floating-point tolerance

Amplitudes are double-precision complex numbers. Diagnostic output rounds
display to 6 decimals and hides amplitudes with magnitude below `1e-9`
(`qsim.TOL`). Tests use `assertAlmostEqual` (7 places) and statistical
tolerances of ±0.1 for sampling frequencies.

## Tests

```
python -m unittest test_qsim -v
```

Covers validation, gate action on the correct amplitude pairs, phase
interference (`H`–`S`–`H`), Bell-state entanglement and correlated
mid-circuit measurement, classical feedback conditioning, per-shot state
reset, and seed reproducibility.
