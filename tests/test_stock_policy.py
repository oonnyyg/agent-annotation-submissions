import json
import subprocess
import sys
import unittest
from decimal import Decimal
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from stock_policy.core import InputError, Item, expected_metrics, solve_budget
from stock_policy.io import parse_document


def make_item(item_id, price, cost, salvage, penalty, demand):
    return Item(item_id, price, cost, salvage, penalty,
                tuple(sorted((q, Decimal(p)) for q, p in demand.items())))


class ExpectedMetricsTest(unittest.TestCase):
    def test_distribution_not_mean(self):
        item = make_item("x", 1000, 600, 100, 50,
                         {0: "0.2", 1: "0.5", 2: "0.3"})
        m = expected_metrics(item, 1)
        self.assertEqual(m.expected_sales, Decimal("0.8"))
        self.assertEqual(m.expected_leftover, Decimal("0.2"))
        self.assertEqual(m.expected_shortage, Decimal("0.3"))
        self.assertEqual(m.sales_revenue_cents, Decimal("800"))
        self.assertEqual(m.salvage_revenue_cents, Decimal("20"))
        self.assertEqual(m.purchase_cost_cents, Decimal("600"))
        self.assertEqual(m.shortage_penalty_cents, Decimal("15"))
        self.assertEqual(m.expected_profit_cents, Decimal("205"))

    def test_exact_decimal_probabilities(self):
        item = make_item("x", 3, 1, 0, 0, {1: "0.1", 2: "0.9"})
        m = expected_metrics(item, 2)
        self.assertEqual(m.expected_sales, Decimal("1.9"))
        self.assertEqual(m.expected_profit_cents, Decimal("3.7"))


class SolveBudgetTest(unittest.TestCase):
    def test_zero_budget(self):
        item = make_item("a", 1000, 600, 0, 50, {1: "1"})
        sol = solve_budget([item], 0)
        self.assertEqual(sol.total_cost_cents, 0)
        self.assertEqual(dict((i.item_id, m.quantity)
                              for i, m in sol.choices), {"a": 0})
        self.assertEqual(sol.total_expected_profit_cents, Decimal("-50"))

    def test_salvage_makes_overstock_harmless(self):
        item = make_item("b", 500, 300, 300, 0, {1: "0.5", 2: "0.5"})
        sol = solve_budget([item], 600)
        qty = sol.choices[0][1].quantity
        self.assertEqual(qty, 2)
        self.assertEqual(sol.total_expected_profit_cents, Decimal("300"))

    def test_greedy_ratio_fails(self):
        # Item "fat": best ratio per cent but blocks the better pair.
        fat = make_item("fat", 100, 60, 0, 0, {1: "1"})      # profit 40, ratio 2/3
        slim1 = make_item("s1", 100, 50, 0, 0, {1: "1"})     # profit 50, ratio 1
        slim2 = make_item("s2", 100, 50, 0, 0, {1: "1"})
        sol = solve_budget([fat, slim1, slim2], 100)
        chosen = {i.item_id: m.quantity for i, m in sol.choices}
        self.assertEqual(chosen, {"fat": 0, "s1": 1, "s2": 1})
        self.assertEqual(sol.total_expected_profit_cents, Decimal("100"))

    def test_input_order_does_not_matter(self):
        a = make_item("a", 1000, 600, 100, 50, {0: "0.2", 1: "0.5", 2: "0.3"})
        b = make_item("b", 500, 300, 300, 0, {1: "0.5", 2: "0.5"})
        sol1 = solve_budget([a, b], 1200)
        sol2 = solve_budget([b, a], 1200)
        self.assertEqual([i.item_id for i, _ in sol1.choices], ["a", "b"])
        self.assertEqual(sol1, sol2)

    def test_tie_prefers_less_capital(self):
        cheap = make_item("c", 100, 50, 50, 0, {1: "1"})   # q=1 profit 50
        dear = make_item("d", 150, 100, 100, 0, {1: "1"})  # q=1 profit 50
        sol = solve_budget([cheap, dear], 100)
        self.assertEqual(sol.total_cost_cents, 50)
        chosen = {i.item_id: m.quantity for i, m in sol.choices}
        self.assertEqual(chosen, {"c": 1, "d": 0})

    def test_tie_lexicographic_vector(self):
        # Two identical items, budget for one unit: vector (0,1) < (1,0)?
        # Lexicographically smaller ordered by id means item "a" gets 0.
        a = make_item("a", 100, 50, 0, 0, {1: "1"})
        b = make_item("b", 100, 50, 0, 0, {1: "1"})
        sol = solve_budget([a, b], 50)
        chosen = tuple(m.quantity for _, m in sol.choices)
        self.assertEqual(chosen, (0, 1))


class ParseTest(unittest.TestCase):
    def base_doc(self):
        return {
            "items": [{
                "id": "a", "price_cents": 1000, "cost_cents": 600,
                "salvage_cents": 100, "shortage_penalty_cents": 50,
                "demand": {"0": "0.2", "1": "0.8"},
            }],
            "budgets": [600],
        }

    def test_duplicate_quantities_merged(self):
        doc = self.base_doc()
        doc["items"][0]["demand"] = [
            {"quantity": 1, "probability": "0.3"},
            {"quantity": 1, "probability": "0.5"},
            {"quantity": 0, "probability": "0.2"},
        ]
        items, _ = parse_document(doc)
        self.assertEqual(items[0].demand,
                         ((0, Decimal("0.2")), (1, Decimal("0.8"))))

    def test_probabilities_must_sum_to_one(self):
        doc = self.base_doc()
        doc["items"][0]["demand"] = {"1": "0.9999999999"}
        with self.assertRaises(InputError):
            parse_document(doc)

    def test_salvage_above_cost_rejected(self):
        doc = self.base_doc()
        doc["items"][0]["salvage_cents"] = 601
        with self.assertRaises(InputError):
            parse_document(doc)

    def test_price_below_cost_rejected(self):
        doc = self.base_doc()
        doc["items"][0]["price_cents"] = 599
        with self.assertRaises(InputError):
            parse_document(doc)

    def test_non_string_probability_rejected(self):
        doc = self.base_doc()
        doc["items"][0]["demand"] = {"1": 1.0}
        with self.assertRaises(InputError):
            parse_document(doc)


class CliTest(unittest.TestCase):
    def run_cli(self, tmp: Path, doc, *extra):
        src = tmp / "in.json"
        src.write_text(json.dumps(doc), encoding="utf-8")
        return subprocess.run(
            [sys.executable, "-m", "stock_policy", str(src), *extra],
            capture_output=True, text=True,
            cwd=Path(__file__).resolve().parent.parent)

    def test_invalid_input_writes_nothing(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            doc = {"items": [], "budgets": [100]}
            out_json = tmp / "plan.json"
            out_csv = tmp / "items.csv"
            result = self.run_cli(tmp, doc, "--json-out", str(out_json),
                                  "--csv-out", str(out_csv))
            self.assertEqual(result.returncode, 2)
            self.assertIn("invalid input", result.stderr)
            self.assertFalse(out_json.exists())
            self.assertFalse(out_csv.exists())

    def test_multiple_budgets_independent(self):
        import tempfile
        with tempfile.TemporaryDirectory() as td:
            tmp = Path(td)
            doc = {
                "items": [{
                    "id": "a", "price_cents": 1000, "cost_cents": 600,
                    "salvage_cents": 0, "shortage_penalty_cents": 0,
                    "demand": {"1": "1"},
                }],
                "budgets": [{"name": "no", "budget_cents": 0},
                            {"name": "yes", "budget_cents": 600}],
            }
            result = self.run_cli(tmp, doc)
            self.assertEqual(result.returncode, 0, result.stderr)
            plans = json.loads(result.stdout)["plans"]
            self.assertEqual([p["items"][0]["quantity"] for p in plans], [0, 1])


if __name__ == "__main__":
    unittest.main()
