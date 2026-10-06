"""Tests for formula parsing, balancing and the operation engine."""

from __future__ import annotations

import json
import subprocess
import sys
import unittest
from fractions import Fraction
from pathlib import Path

from reaction_balance.balance import (
    NoSolutionError,
    NotUniqueError,
    balance_equation,
)
from reaction_balance.engine import run_operations
from reaction_balance.formula import FormulaError, parse_formula
from reaction_balance.model import load_definition


class FormulaTests(unittest.TestCase):
    def test_simple(self):
        self.assertEqual(parse_formula("H2O"), {"H": 2, "O": 1})

    def test_nested_parentheses(self):
        self.assertEqual(
            parse_formula("K4(ON(SO3)2)2"),
            {"K": 4, "O": 14, "N": 2, "S": 4},
        )

    def test_unknown_element(self):
        with self.assertRaises(FormulaError):
            parse_formula("Xx2")

    def test_bad_parentheses(self):
        for bad in ("(H2O", "H2O)", "H2(O)2)", "()"):
            with self.assertRaises(FormulaError, msg=bad):
                parse_formula(bad)

    def test_unsupported_notations(self):
        for bad in ("Na+", "CuSO4.5H2O", "13C", "H2O0"):
            with self.assertRaises(FormulaError, msg=bad):
                parse_formula(bad)


class BalanceTests(unittest.TestCase):
    def test_parenthesised_equation(self):
        coeffs = balance_equation(
            ["Ca(OH)2", "H3PO4"], ["Ca3(PO4)2", "H2O"]
        )
        self.assertEqual(
            coeffs, {"Ca(OH)2": 3, "H3PO4": 2, "Ca3(PO4)2": 1, "H2O": 6}
        )

    def test_not_unique(self):
        # C + O2 -> CO + CO2 has a 2-dimensional null space.
        with self.assertRaises(NotUniqueError):
            balance_equation(["C", "O2"], ["CO", "CO2"])

    def test_no_solution(self):
        with self.assertRaises(NoSolutionError):
            balance_equation(["H2"], ["O2"])

    def test_duplicate_species_rejected(self):
        from reaction_balance.balance import BalanceError

        with self.assertRaises(BalanceError):
            balance_equation(["H2", "H2"], ["H2O"])


def make_definition() -> dict:
    return {
        "reactions": [
            {
                "name": "synthesize_water",
                "reactants": ["H2", "O2"],
                "products": ["H2O"],
            },
            {
                "name": "electrolyze",
                "reactants": ["H2O"],
                "products": ["H2", "O2"],
            },
        ],
        "operations": [
            {"type": "add", "species": "H2", "amount": "5"},
            {"type": "add", "species": "O2", "amount": "2"},
            {"type": "react", "reaction": "synthesize_water", "fraction": "0.5"},
            {"type": "react", "reaction": "electrolyze", "fraction": "1"},
        ],
    }


class EngineTests(unittest.TestCase):
    def test_product_reused_in_later_step(self):
        reactions, operations = load_definition(make_definition())
        steps = run_operations(reactions, operations)
        # Step 2: max extent = min(5/2, 2/1) = 2, extent = 1.
        step2 = steps[2]
        self.assertEqual(step2.consumed, {"H2": Fraction(2), "O2": Fraction(1)})
        self.assertEqual(step2.produced, {"H2O": Fraction(2)})
        self.assertEqual(step2.limiting_reactants, ["O2"])
        # Step 3: electrolyze all 2 mol H2O back.
        step3 = steps[3]
        self.assertEqual(step3.consumed, {"H2O": Fraction(2)})
        self.assertEqual(step3.produced, {"H2": Fraction(2), "O2": Fraction(1)})
        final = step3.inventory
        self.assertEqual(final["H2"], Fraction(5))
        self.assertEqual(final["O2"], Fraction(2))
        self.assertEqual(final["H2O"], Fraction(0))

    def test_decimal_feed_is_exact(self):
        definition = {
            "reactions": [
                {"name": "r", "reactants": ["H2", "O2"], "products": ["H2O"]}
            ],
            "operations": [
                {"type": "add", "species": "H2", "amount": "0.1"}
            ],
        }
        _, operations = load_definition(definition)
        self.assertEqual(operations[0].amount, Fraction(1, 10))

    def test_validation_before_execution(self):
        definition = make_definition()
        definition["operations"].append(
            {"type": "react", "reaction": "missing", "fraction": "1"}
        )
        from reaction_balance.model import DefinitionError

        with self.assertRaises(DefinitionError):
            load_definition(definition)


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            input_path = tmp_path / "input.json"
            input_path.write_text(
                json.dumps(make_definition()), encoding="utf-8"
            )
            balance_json = tmp_path / "balance.json"
            steps_csv = tmp_path / "steps.csv"
            result = subprocess.run(
                [
                    sys.executable, "-m", "reaction_balance",
                    str(input_path),
                    "--balance-json", str(balance_json),
                    "--steps-csv", str(steps_csv),
                ],
                capture_output=True,
                text=True,
                cwd=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            report = json.loads(balance_json.read_text(encoding="utf-8"))
            self.assertEqual(
                report["reactions"][0]["coefficients"],
                {"H2": 2, "O2": 1, "H2O": 2},
            )
            self.assertEqual(
                report["steps"][2]["limiting_reactants"], ["O2"]
            )
            rows = steps_csv.read_text(encoding="utf-8").splitlines()
            self.assertEqual(len(rows), 1 + 4)  # header + 4 steps

    def test_error_leaves_no_output(self):
        import tempfile

        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            bad = make_definition()
            bad["reactions"].append(
                {"name": "bad", "reactants": ["C", "O2"],
                 "products": ["CO", "CO2"]}
            )
            input_path = tmp_path / "input.json"
            input_path.write_text(json.dumps(bad), encoding="utf-8")
            balance_json = tmp_path / "balance.json"
            steps_csv = tmp_path / "steps.csv"
            result = subprocess.run(
                [
                    sys.executable, "-m", "reaction_balance",
                    str(input_path),
                    "--balance-json", str(balance_json),
                    "--steps-csv", str(steps_csv),
                ],
                capture_output=True,
                text=True,
                cwd=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(result.returncode, 1)
            self.assertIn("not unique", result.stderr)
            self.assertFalse(balance_json.exists())
            self.assertFalse(steps_csv.exists())


if __name__ == "__main__":
    unittest.main()