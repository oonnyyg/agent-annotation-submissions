"""Command-line interface for the Clifford shot simulator.

Usage:
    python cli.py circuit.json --seed 42            # single-shot diagnostic
    python cli.py circuit.json --shots 1000 --seed 42
"""

from __future__ import annotations

import argparse
import json
import random
import sys

import qsim


def _format_amplitude(a: complex) -> str:
    re, im = a.real, a.imag
    if abs(re) < qsim.TOL:
        re = 0.0
    if abs(im) < qsim.TOL:
        im = 0.0
    return f"{re:+.6f}{im:+.6f}j"


def run_diagnostic(circuit: qsim.Circuit, seed: int | None) -> dict:
    result = qsim.run_shot(circuit, random.Random(seed))
    n = circuit.num_qubits
    return {
        "mode": "diagnostic",
        "qubits": n,
        "mid_circuit_measurements": [
            {
                "op_index": r.op_index,
                "qubit": r.qubit,
                "classical_bit": r.classical_bit,
                "outcome": r.outcome,
            }
            for r in result.measurements
        ],
        "classical_bits": result.classical_bits,
        "state_vector": [
            {"index": i, "bits": format(i, f"0{n}b"), "amplitude": _format_amplitude(a)}
            for i, a in enumerate(result.state)
            if abs(a) > qsim.TOL
        ],
        "final_measurement": result.final_bits,
    }


def run_sampling(circuit: qsim.Circuit, shots: int, seed: int | None) -> dict:
    counts = qsim.run_counts(circuit, shots, seed)
    return {
        "mode": "sampling",
        "shots": shots,
        "counts": [
            {"bitstring": bits, "count": c, "frequency": c / shots}
            for bits, c in sorted(counts.items())
        ],
    }


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Clifford shot simulator (no quantum hardware).")
    parser.add_argument("circuit", help="path to the JSON circuit file")
    parser.add_argument("--shots", type=int, default=None,
                        help="number of sampling shots; omit for single-shot diagnostic")
    parser.add_argument("--seed", type=int, default=None, help="RNG seed for reproducibility")
    args = parser.parse_args(argv)

    try:
        with open(args.circuit, "r", encoding="utf-8") as fh:
            data = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read circuit file: {exc}", file=sys.stderr)
        return 2

    try:
        circuit = qsim.parse_circuit(data)
    except qsim.CircuitError as exc:
        print(f"error: invalid circuit: {exc}", file=sys.stderr)
        return 2

    if args.shots is None:
        report = run_diagnostic(circuit, args.seed)
    else:
        if args.shots <= 0:
            print("error: --shots must be positive", file=sys.stderr)
            return 2
        report = run_sampling(circuit, args.shots, args.seed)

    json.dump(report, sys.stdout, indent=2)
    print()
    return 0


if __name__ == "__main__":
    sys.exit(main())
