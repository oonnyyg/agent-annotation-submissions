"""CLI: python -m reaction_balance input.json [--csv steps.csv] [--json report.json]"""

from __future__ import annotations

import argparse
import csv
import json
import os
import sys
from fractions import Fraction

from .balance import BalanceError
from .engine import DefinitionError, load_definition, simulate
from .formula import FormulaError

CSV_DECIMAL_PLACES = 10


def _exact(value: Fraction) -> str:
    if value.denominator == 1:
        return str(value.numerator)
    return f"{value.numerator}/{value.denominator}"


def _decimal(value: Fraction) -> str:
    if value.denominator == 1:
        return str(value.numerator)
    rounded = round(float(value), CSV_DECIMAL_PLACES)
    return f"{rounded:.{CSV_DECIMAL_PLACES}f}".rstrip("0").rstrip(".")


def _build_report(reactions, steps):
    return {
        "reactions": [
            {
                "name": r["name"],
                "reactants": r["reactants"],
                "products": r["products"],
                "coefficients": r["coefficients"],
            }
            for r in reactions
        ],
        "steps": [
            {
                "step": s["step"],
                "action": s["action"],
                "detail": s["detail"],
                "changes": {k: _exact(v) for k, v in s["changes"].items()},
                "limiting_reactants": s["limiting_reactants"],
                "stock": {k: _exact(v) for k, v in s["stock"].items()},
            }
            for s in steps
        ],
    }


def _write_csv(path, steps):
    directory = os.path.dirname(os.path.abspath(path))
    os.makedirs(directory, exist_ok=True)
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            ["step", "action", "detail", "species", "change_mol",
             "balance_mol", "limiting_reactants"]
        )
        for s in steps:
            limiting = ";".join(s["limiting_reactants"])
            species_set = sorted(set(s["changes"]) | set(s["stock"]))
            if not species_set:
                writer.writerow([s["step"], s["action"], s["detail"], "", "", "", limiting])
            for species in species_set:
                change = s["changes"].get(species)
                balance = s["stock"].get(species, Fraction(0))
                writer.writerow(
                    [
                        s["step"],
                        s["action"],
                        s["detail"],
                        species,
                        "" if change is None else _decimal(change),
                        _decimal(balance),
                        limiting,
                    ]
                )


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="reaction_balance",
        description="Balance reactions and simulate feed operations exactly.",
    )
    parser.add_argument("input", help="JSON file with 'reactions' and 'operations'")
    parser.add_argument("--csv", dest="csv_path", help="write step CSV to this path")
    parser.add_argument("--json", dest="json_path",
                        help="write JSON report to this path instead of stdout")
    args = parser.parse_args(argv)

    try:
        with open(args.input, encoding="utf-8-sig") as fh:
            data = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2

    try:
        reactions, operations = load_definition(data)
        steps = simulate(reactions, operations)
    except (DefinitionError, BalanceError, FormulaError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    report = _build_report(reactions, steps)
    text = json.dumps(report, indent=2, ensure_ascii=False)
    if args.csv_path:
        _write_csv(args.csv_path, steps)
    if args.json_path:
        os.makedirs(os.path.dirname(os.path.abspath(args.json_path)), exist_ok=True)
        with open(args.json_path, "w", encoding="utf-8") as fh:
            fh.write(text + "\n")
    else:
        print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
