import json
import math
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from uncertainty_budget.errors import EvaluationError, ExpressionError, InputSpecError
from uncertainty_budget.evaluate import evaluate
from uncertainty_budget.model import load_correlation, load_inputs, load_results
from uncertainty_budget.parser import parse
from uncertainty_budget.propagate import compute


def run_spec(spec):
    inputs = load_inputs(spec)
    corr = load_correlation(spec, len(inputs))
    results = load_results(spec, {x["name"] for x in inputs})
    return compute(inputs, corr, results)


class ParserTests(unittest.TestCase):
    def test_basic_arithmetic(self):
        val, der = evaluate(parse("a + 2*b - (a*b)^2 / 4"), {"a": 3.0, "b": 2.0})
        self.assertAlmostEqual(val, 3 + 4 - 9)
        self.assertAlmostEqual(der["a"], 1 - 2 * 3 * 4 / 4)
        self.assertAlmostEqual(der["b"], 2 - 2 * 6 * 3 / 4)

    def test_functions(self):
        val, der = evaluate(parse("sqrt(x) + exp(y) + ln(x)"), {"x": 4.0, "y": 1.0})
        self.assertAlmostEqual(val, 2 + math.e + math.log(4))
        self.assertAlmostEqual(der["x"], 0.25 + 0.25)
        self.assertAlmostEqual(der["y"], math.e)

    def test_undeclared_variable_rejected(self):
        with self.assertRaises(ExpressionError):
            evaluate(parse("a + c"), {"a": 1.0, "b": 2.0})

    def test_syntax_error_positioned(self):
        with self.assertRaises(ExpressionError) as ctx:
            parse("a + * b")
        self.assertIn("column", str(ctx.exception))

    def test_non_integer_exponent_rejected(self):
        with self.assertRaises(ExpressionError):
            parse("x^1.5")

    def test_unknown_function_rejected(self):
        with self.assertRaises(ExpressionError):
            parse("sin(x)")

    def test_domain_errors(self):
        with self.assertRaises(EvaluationError):
            evaluate(parse("sqrt(x)"), {"x": -1.0})
        with self.assertRaises(EvaluationError):
            evaluate(parse("sqrt(x)"), {"x": 0.0})
        with self.assertRaises(EvaluationError):
            evaluate(parse("ln(x)"), {"x": 0.0})
        with self.assertRaises(EvaluationError):
            evaluate(parse("1 / x"), {"x": 0.0})
        with self.assertRaises(EvaluationError):
            evaluate(parse("x^-1"), {"x": 0.0})

    def test_no_arbitrary_code(self):
        for bad in ["__import__('os')", "open('f')", "x; y", "[1,2]", "{'a':1}"]:
            with self.assertRaises(ExpressionError, msg=bad):
                parse(bad)


class CorrelationTests(unittest.TestCase):
    def test_identity_default(self):
        corr = load_correlation({"inputs": [1, 2]}, 2)
        self.assertEqual(corr, [[1.0, 0.0], [0.0, 1.0]])

    def test_wrong_dimension(self):
        with self.assertRaises(InputSpecError):
            load_correlation({"correlation": [[1.0]]}, 2)

    def test_non_unit_diagonal(self):
        with self.assertRaises(InputSpecError):
            load_correlation({"correlation": [[1.0, 0.0], [0.0, 0.5]]}, 2)

    def test_asymmetric(self):
        with self.assertRaises(InputSpecError):
            load_correlation({"correlation": [[1.0, 0.3], [0.2, 1.0]]}, 2)

    def test_each_entry_valid_but_matrix_not_psd(self):
        # Every coefficient is in [-1, 1] yet the matrix is indefinite.
        bad = [[1.0, -0.9, 0.9], [-0.9, 1.0, 0.9], [0.9, 0.9, 1.0]]
        with self.assertRaises(InputSpecError):
            load_correlation({"correlation": bad}, 3)

    def test_singular_but_valid_accepted(self):
        ok = [[1.0, -1.0], [-1.0, 1.0]]
        self.assertEqual(load_correlation({"correlation": ok}, 2), ok)
        ok3 = [[1.0, 1.0, -1.0], [1.0, 1.0, -1.0], [-1.0, -1.0, 1.0]]
        self.assertEqual(load_correlation({"correlation": ok3}, 3), ok3)


class PropagationTests(unittest.TestCase):
    def test_shared_input_perfect_correlation(self):
        # y = a - b with identical, fully correlated inputs cancels exactly.
        spec = {
            "inputs": [
                {"name": "a", "value": 5.0, "std_uncertainty": 0.3},
                {"name": "b", "value": 5.0, "std_uncertainty": 0.3},
            ],
            "correlation": [[1.0, 1.0], [1.0, 1.0]],
            "results": [{"name": "y", "expression": "a - b"}],
        }
        out = run_spec(spec)
        res = out["results"][0]
        self.assertEqual(res["std_uncertainty"], 0.0)
        total = sum(t["contribution"] for t in res["budget"])
        self.assertAlmostEqual(total, res["variance"])
        cross = [t for t in res["budget"] if t["type"] == "cross"][0]
        self.assertLess(cross["contribution"], 0.0)

    def test_negative_correlation_cancellation(self):
        # y = a + b with rho = -1 gives zero total variance (singular matrix).
        spec = {
            "inputs": [
                {"name": "a", "value": 1.0, "std_uncertainty": 0.2},
                {"name": "b", "value": 2.0, "std_uncertainty": 0.2},
            ],
            "correlation": [[1.0, -1.0], [-1.0, 1.0]],
            "results": [{"name": "y", "expression": "a + b"}],
        }
        out = run_spec(spec)
        self.assertEqual(out["results"][0]["std_uncertainty"], 0.0)

    def test_independent_sum_of_squares(self):
        spec = {
            "inputs": [
                {"name": "a", "value": 1.0, "std_uncertainty": 0.3},
                {"name": "b", "value": 2.0, "std_uncertainty": 0.4},
            ],
            "results": [{"name": "y", "expression": "a + b"}],
        }
        out = run_spec(spec)
        self.assertAlmostEqual(out["results"][0]["std_uncertainty"], 0.5)

    def test_result_correlation_shared_inputs(self):
        spec = {
            "inputs": [
                {"name": "a", "value": 1.0, "std_uncertainty": 0.1},
                {"name": "b", "value": 1.0, "std_uncertainty": 0.1},
            ],
            "results": [
                {"name": "s", "expression": "a + b"},
                {"name": "d", "expression": "a - b"},
            ],
        }
        out = run_spec(spec)
        self.assertAlmostEqual(out["result_correlation"][0][1], 0.0, places=12)
        spec["results"][1]["expression"] = "a + 2*b"
        out = run_spec(spec)
        r = out["result_correlation"][0][1]
        self.assertTrue(0.0 < r < 1.0)

    def test_zero_uncertainty_result_correlation_is_null(self):
        spec = {
            "inputs": [{"name": "a", "value": 1.0, "std_uncertainty": 0.0}],
            "results": [
                {"name": "x", "expression": "a"},
                {"name": "c", "expression": "2"},
            ],
        }
        out = run_spec(spec)
        self.assertIsNone(out["result_correlation"][0][0])
        self.assertIsNone(out["result_correlation"][0][1])

    def test_budget_sums_to_variance(self):
        spec = {
            "inputs": [
                {"name": "a", "value": 2.0, "std_uncertainty": 0.1},
                {"name": "b", "value": 3.0, "std_uncertainty": 0.2},
                {"name": "c", "value": 4.0, "std_uncertainty": 0.3},
            ],
            "correlation": [[1.0, 0.5, -0.3], [0.5, 1.0, 0.1], [-0.3, 0.1, 1.0]],
            "results": [{"name": "y", "expression": "a*b - c^2 + ln(c)"}],
        }
        out = run_spec(spec)
        res = out["results"][0]
        total = sum(t["contribution"] for t in res["budget"])
        self.assertAlmostEqual(total, res["variance"], places=12)


class CliTests(unittest.TestCase):
    def test_cli_end_to_end(self):
        spec = {
            "inputs": [
                {"name": "a", "value": 10.0, "std_uncertainty": 0.2},
                {"name": "b", "value": 9.9, "std_uncertainty": 0.2},
            ],
            "correlation": [[1.0, -1.0], [-1.0, 1.0]],
            "results": [{"name": "total", "expression": "a + b"}],
        }
        with tempfile.TemporaryDirectory() as tmp:
            spec_path = Path(tmp, "spec.json")
            csv_path = Path(tmp, "budget.csv")
            spec_path.write_text(json.dumps(spec), encoding="utf-8")
            proc = subprocess.run(
                [sys.executable, "-m", "uncertainty_budget", str(spec_path),
                 "--csv", str(csv_path)],
                capture_output=True, text=True,
                cwd=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            data = json.loads(proc.stdout)
            self.assertEqual(data["results"][0]["std_uncertainty"], 0.0)
            rows = csv_path.read_text(encoding="utf-8").strip().splitlines()
            self.assertEqual(len(rows), 1 + 3)  # header + 2 diagonal + 1 cross
            self.assertIn("-0.08", rows[3])

    def test_cli_reports_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            spec_path = Path(tmp, "spec.json")
            spec_path.write_text(json.dumps({
                "inputs": [{"name": "a", "value": 1.0, "std_uncertainty": -0.1}],
                "results": [{"name": "y", "expression": "a"}],
            }), encoding="utf-8")
            proc = subprocess.run(
                [sys.executable, "-m", "uncertainty_budget", str(spec_path)],
                capture_output=True, text=True,
                cwd=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(proc.returncode, 2)
            self.assertIn("error:", proc.stderr)


if __name__ == "__main__":
    unittest.main()
