from __future__ import annotations

import argparse
import json
import sys
from collections.abc import Sequence
from dataclasses import asdict
from pathlib import Path

from .circuit import CircuitError, run_diagnostic, sample_circuit


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(description="Simulate a small Clifford quantum circuit.")
    parser.add_argument("circuit", type=Path, help="path to a JSON circuit file")
    parser.add_argument("--shots", type=int, default=1, help="number of independent samples (default: 1)")
    parser.add_argument("--seed", type=int, default=None, help="deterministic random seed")
    args = parser.parse_args(argv)

    try:
        with args.circuit.open("r", encoding="utf-8") as circuit_file:
            circuit = json.load(circuit_file)
        if args.shots == 1:
            result = asdict(run_diagnostic(circuit, args.seed))
        else:
            result = asdict(sample_circuit(circuit, args.shots, args.seed))
    except (OSError, json.JSONDecodeError, CircuitError) as error:
        print(f"error: {error}", file=sys.stderr)
        return 1

    json.dump(result, sys.stdout, ensure_ascii=False, indent=2, allow_nan=False)
    print()
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
