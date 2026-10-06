import json
import subprocess
import sys
import unittest
from decimal import Decimal
from pathlib import Path

from stock_policy.errors import InputError
from stock_policy.model import parse_problem
from stock_policy.profit import evaluate, options_for
from stock_policy.solver import solve

ROOT = Path(__file__).resolve().parent.parent


def make_item(item_id, price, cost, salvage, penalty, demand):
    doc = {
        "items": [{
            "id": item_id, "price": price, "cost": cost, "salvage": salvage,
            "shortage_penalty": penalty,
            "demand": [{"quantity": q, "probability": p} for q, p in demand],
        }],
        "budgets": [0],
    }
    return parse_problem(doc).items[0]


class ProfitTests(unittest.TestCase):
    def test_expected_values_use_full_distribution(self):
        item = make_item("A", 1000, 600, 100, 50,
                         [(0, "0.2"), (1, "0.5"), (2, "0.3")])
        opt = evaluate(item, 1)
        self.assertEqual(opt.expected_sales, Decimal("0.8"))
        self.assertEqual(opt.expected_leftover, Decimal("0.2"))
        self.assertEqual(opt.expected_shortage, Decimal("0.3"))
        self.assertEqual(opt.expected_profit, Decimal(205))

    def test_salvage_rewards_leftover_stock(self):
        item = make_item("A", 500, 400, 400, 0, [(0, "1")])
        # Demand is certainly zero; with full salvage, stocking is free.
        self.assertEqual(evaluate(item, 3).expected_profit, Decimal(0))
        item2 = make_item("A", 500, 400, 100, 0, [(0, "1")])
        self.assertEqual(evaluate(item2, 2).expected_profit, Decimal(-600))

    def test_duplicate_quantities_are_merged(self):
        item = make_item("A", 100, 50, 0, 0,
                         [(1, "0.3"), (1, "0.7")])
        self.assertEqual(item.demand, ((1, Decimal("1.0")),))


class SolverTests(unittest.TestCase):
    def test_zero_budget_forces_zero_stock(self):
        items = (make_item("A", 1000, 600, 100, 50, [(1, "1")]),)
        sol = solve(items, 0)
        self.assertEqual(sol.total_cost, 0)
        self.assertEqual(sol.choices[0][1].quantity, 0)
        self.assertEqual(sol.expected_profit, Decimal(-50))

    def test_greedy_by_margin_fails_but_dp_wins(self):
        # Item X: high unit margin, big cost. Items Y,Z: small margins that
        # together beat X under the budget. A per-unit-margin greedy picks X.
        x = make_item("X", 1000, 600, 0, 0, [(1, "1")])   # profit 400, cost 600
        y = make_item("Y", 700, 400, 0, 0, [(1, "1")])    # profit 300, cost 400
        z = make_item("Z", 700, 400, 0, 0, [(1, "1")])    # profit 300, cost 400
        sol = solve((x, y, z), 800)
        picked = {it.item_id: opt.quantity for it, opt in sol.choices}
        self.assertEqual(picked, {"X": 0, "Y": 1, "Z": 1})
        self.assertEqual(sol.expected_profit, Decimal(600))

    def test_tie_prefers_lower_spend(self):
        # Both quantities give the same expected profit; fewer units cost less.
        item = make_item("A", 500, 400, 400, 0, [(0, "1")])
        sol = solve((item,), 10000)
        self.assertEqual(sol.choices[0][1].quantity, 0)
        self.assertEqual(sol.total_cost, 0)

    def test_tie_prefers_lexicographically_smaller_vector(self):
        # A and B are identical; profit ties at (0,1) vs (1,0) with equal cost.
        a = make_item("A", 500, 400, 400, 0, [(1, "1")])
        b = make_item("B", 500, 400, 400, 0, [(1, "1")])
        sol = solve((a, b), 400)
        picked = tuple(opt.quantity for _, opt in sol.choices)
        self.assertEqual(picked, (0, 1))

    def test_input_order_does_not_change_result(self):
        a = make_item("A", 1000, 600, 100, 50, [(0, "0.5"), (2, "0.5")])
        b = make_item("B", 1500, 900, 200, 0, [(0, "0.4"), (1, "0.6")])
        sol1 = solve((a, b), 1500)
        sol2 = solve((b, a), 1500)
        self.assertEqual(
            [(it.item_id, o.quantity) for it, o in sol1.choices],
            [(it.item_id, o.quantity) for it, o in sol2.choices])
        self.assertEqual(sol1.expected_profit, sol2.expected_profit)

    def test_exact_decimal_probabilities(self):
        # Probabilities with many decimals; sum must be exactly one.
        item = make_item("A", 1000, 600, 0, 0,
                         [(0, "0.3333333333"), (1, "0.6666666667")])
        opt = evaluate(item, 1)
        self.assertEqual(opt.expected_profit,
                         Decimal("0.6666666667") * 1000 - 600)


class ValidationTests(unittest.TestCase):
    def bad(self, doc):
        with self.assertRaises(InputError):
            parse_problem(doc)

    def base_item(self):
        return {"id": "A", "price": 100, "cost": 50, "salvage": 10,
                "shortage_penalty": 0,
                "demand": [{"quantity": 1, "probability": "1"}]}

    def test_probabilities_must_sum_to_one(self):
        item = self.base_item()
        item["demand"] = [{"quantity": 1, "probability": "0.5"}]
        self.bad({"items": [item], "budgets": [0]})

    def test_price_below_cost_rejected(self):
        item = self.base_item()
        item["price"] = 40
        self.bad({"items": [item], "budgets": [0]})

    def test_salvage_above_cost_rejected(self):
        item = self.base_item()
        item["salvage"] = 60
        self.bad({"items": [item], "budgets": [0]})

    def test_negative_penalty_rejected(self):
        item = self.base_item()
        item["shortage_penalty"] = -1
        self.bad({"items": [item], "budgets": [0]})

    def test_non_string_probability_rejected(self):
        item = self.base_item()
        item["demand"] = [{"quantity": 1, "probability": 1.0}]
        self.bad({"items": [item], "budgets": [0]})


class CliTests(unittest.TestCase):
    def run_cli(self, *args):
        return subprocess.run(
            [sys.executable, "-m", "stock_policy", *args],
            cwd=ROOT, capture_output=True, text=True)

    def test_invalid_input_produces_no_output_files(self):
        bad = ROOT / "tests" / "_bad.json"
        out = ROOT / "tests" / "_out.json"
        bad.write_text(json.dumps(
            {"items": [], "budgets": [0]}), encoding="utf-8")
        try:
            proc = self.run_cli(str(bad), "--json", str(out))
            self.assertNotEqual(proc.returncode, 0)
            self.assertFalse(out.exists())
        finally:
            bad.unlink(missing_ok=True)
            out.unlink(missing_ok=True)

    def test_sample_runs_end_to_end(self):
        proc = self.run_cli(str(ROOT / "examples" / "sample.json"))
        self.assertEqual(proc.returncode, 0, proc.stderr)
        doc = json.loads(proc.stdout)
        self.assertEqual(len(doc["budgets"]), 4)


if __name__ == "__main__":
    unittest.main()
