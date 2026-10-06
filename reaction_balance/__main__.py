"""Command line interface: python -m reaction_balance.

Reads a JSON file with reaction definitions and an operation sequence,
writes a JSON report of the balancing results and a CSV of the stepwise
material balance. All definitions are validated before any output file
is written, so errors never leave partial or fake-complete results.
"""

from __future__ import annotations

import argparse
import csv
import json
import sys
from fractions import Fraction
from pathlib import Path

from .balance import BalanceError
from .engine import ExecutionError, run_operations
from .formula import FormulaError
from .model import DefinitionError, StepRecord, load_definition

OUTPUT_DECIMALS = 6


def format_amount(value: Fraction) -> str:
    """Exact string for integers, fixed-precision decimal otherwise."""
    if value.denominator == 1:
        return str(value.numerator)
    return f"{float(value):.{OUTPUT_DECIMALS}f}"


def build_report(reactions, steps: list[StepRecord]) -> dict:
    return {
        "reactions": [
            {
                "name": reaction.name,
                "reactants": reaction.reactants,
                "products": reaction.products,
                "coefficients": reaction.coefficients,
            }
            for reaction in reactions
        ],
        "steps": [
            {
                "index": step.index,
                "type": step.kind,
                "description": step.description,
                "consumed": {k: format_amount(v) for k, v in step.consumed.items()},
                "produced": {k: format_amount(v) for k, v in step.produced.items()},
                "limiting_reactants": step.limiting_reactants,
                "inventory": {
                    k: format_amount(v) for k, v in sorted(step.inventory.items())
                },
            }
            for step in steps
        ],
    }


def write_steps_csv(steps: list[StepRecord], path: Path) -> None:
    species: list[str] = sorted(
        {name for step in steps for name in step.inventory}
    )
    with path.open("w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            ["step", "type", "description", "consumed", "produced",
             "limiting_reactants"]
            + [f"remaining[{name}]" for name in species]
        )
        for step in steps:
            consumed = "; ".join(
                f"{k}: {format_amount(v)}" for k, v in step.consumed.items()
            )
            produced = "; ".join(
                f"{k}: {format_amount(v)}" for k, v in step.produced.items()
            )
            writer.writerow(
                [step.index, step.kind, step.description, consumed, produced,
                 "; ".join(step.limiting_reactants)]
                + [format_amount(step.inventory.get(name, Fraction(0)))
                   for name in species]
            )


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m reaction_balance",
        description="Stoichiometric feed balance: balance equations and "
        "track inventory through feed/reaction steps.",
    )
    parser.add_argument("input", help="JSON definition file ('-' for stdin)")
    parser.add_argument(
        "--balance-json",
        default="balance.json",
        help="output path for the balancing/step report JSON",
    )
    parser.add_argument(
        "--steps-csv",
        default="steps.csv",
        help="output path for the stepwise material CSV",
    )
    args = parser.parse_args(argv)

    try:
        if args.input == "-":
            data = json.load(sys.stdin)
        else:
            with open(args.input, encoding="utf-8") as handle:
                data = json.load(handle)
        reactions, operations = load_definition(data)
        steps = run_operations(reactions, operations)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2
    except (DefinitionError, FormulaError, BalanceError, ExecutionError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    report = build_report(reactions, steps)
    balance_path = Path(args.balance_json)
    balance_path.write_text(
        json.dumps(report, indent=2, ensure_ascii=False) + "\n",
        encoding="utf-8",
    )
    write_steps_csv(steps, Path(args.steps_csv))
    print(f"wrote {balance_path} and {args.steps_csv}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())