import json
import math
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from uncertainty_budget.errors import ExpressionError, InputError
from uncertainty_budget.model import load_quantities
from uncertainty_budget.report import budget_csv, compute, report_dict


def run(spec):
    return compute(spec)


def base_spec(**overrides):
    spec = {
        "quantities": [
            {"name": "a", "nominal": 10.0, "std_uncertainty": 0.5},
            {"name": "b", "nominal": 9.0, "std_uncertainty": 0.5},
        ],
        "results": [{"name": "s", "expression": "a + b"}],
    }
    spec.update(overrides)
    return spec


class TestCorrelationMatrix(unittest.TestCase):
    def test_omitted_means_independent(self):
        _, corr, budgets, _, _ = run(base_spec())
        self.assertEqual(corr, [[1.0, 0.0], [0.0, 1.0]])
        self.assertAlmostEqual(budgets[0].variance, 0.5)

    def test_wrong_dimension_rejected(self):
        with self.assertRaises(InputError):
            run(base_spec(correlation=[[1.0]]))

    def test_non_unit_diagonal_rejected(self):
        with self.assertRaises(InputError):
            run(base_spec(correlation=[[1.0, 0.0], [0.0, 0.5]]))

    def test_asymmetry_rejected(self):
        with self.assertRaises(InputError):
            run(base_spec(correlation=[[1.0, 0.3], [0.2, 1.0]]))

    def test_coefficient_out_of_range_rejected(self):
        with self.assertRaises(InputError):
            run(base_spec(correlation=[[1.0, 1.5], [1.5, 1.0]]))

    def test_individually_valid_but_not_psd_rejected(self):
        # Every coefficient is in [-1, 1] yet the matrix is indefinite.
        bad = [[1.0, -0.9, -0.9], [-0.9, 1.0, -0.9], [-0.9, -0.9, 1.0]]
        spec = base_spec(correlation=bad)
        spec["quantities"].append(
            {"name": "c", "nominal": 1.0, "std_uncertainty": 0.1})
        with self.assertRaises(InputError):
            run(spec)


class TestExpressions(unittest.TestCase):
    def test_shared_inputs_and_result_correlation(self):
        spec = base_spec()
        spec["results"] = [
            {"name": "s", "expression": "a + b"},
            {"name": "d", "expression": "a - b"},
        ]
        _, _, budgets, result_cov, corr = run(spec)
        self.assertAlmostEqual(budgets[0].value, 19.0)
        self.assertAlmostEqual(budgets[1].value, 1.0)
        # Equal uncertainties: Var(a+b)=0.5, Var(a-b)=0.5, Cov=0.
        self.assertAlmostEqual(result_cov[0][1], 0.0, places=12)
        self.assertAlmostEqual(corr[0][1], 0.0, places=12)

    def test_functions_and_power(self):
        spec = base_spec()
        spec["results"] = [
            {"name": "p", "expression": "a^2 + sqrt(b)"},
            {"name": "e", "expression": "exp(ln(a)) - a"},
            {"name": "n", "expression": "b^-1"},
        ]
        _, _, budgets, _, _ = run(spec)
        self.assertAlmostEqual(budgets[0].value, 100.0 + 3.0)
        self.assertAlmostEqual(budgets[0].sensitivities[0], 20.0)
        self.assertAlmostEqual(budgets[1].value, 0.0, places=12)
        self.assertAlmostEqual(budgets[2].value, 1.0 / 9.0)
        self.assertAlmostEqual(budgets[2].sensitivities[1], -1.0 / 81.0)

    def test_undeclared_name_rejected_with_location(self):
        spec = base_spec()
        spec["results"] = [{"name": "s", "expression": "a + zz"}]
        with self.assertRaises(ExpressionError) as ctx:
            run(spec)
        self.assertIn("zz", str(ctx.exception))
        self.assertIsNotNone(ctx.exception.position)

    def test_result_cannot_reference_result(self):
        spec = base_spec()
        spec["results"] = [
            {"name": "s", "expression": "a + b"},
            {"name": "t", "expression": "2 * s"},
        ]
        with self.assertRaises(ExpressionError):
            run(spec)


class TestBudgetCsv(unittest.TestCase):
    def test_budget_sums_to_propagated_variance(self):
        spec = base_spec(correlation=[[1.0, -0.9], [-0.9, 1.0]])
        spec["results"] = [
            {"name": "s", "expression": "a + b"},
            {"name": "d", "expression": "a - b"},
        ]
        _, _, budgets, _, _ = run(spec)
        text = budget_csv(spec_quantities(spec), budgets)
        rows = list(line.split(",") for line in text.strip().splitlines())
        header = rows[0]
        self.assertEqual(header[0], "result")
        by_result = {}
        for row in rows[1:]:
            by_result.setdefault(row[0], {})[row[1]] = row
        for name, terms in by_result.items():
            total = sum(
                float(r[4]) for r in rows[1:]
                if r[0] == name and r[1] in ("diagonal", "cross"))
            self.assertAlmostEqual(
                total, float(terms["propagated_variance"][4]), places=12)
            self.assertAlmostEqual(
                float(terms["sum"][4]),
                float(terms["propagated_variance"][4]), places=12)
        # Negative cross term kept (cancellation), not absolutized.
        s_cross = [r for r in rows[1:]
                   if r[0] == "s" and r[1] == "cross" and r[2] == "a"]
        self.assertAlmostEqual(float(s_cross[0][4]), -0.45)

    def test_zero_uncertainty_result_correlation_is_null(self):
        spec = base_spec()
        spec["quantities"][1]["std_uncertainty"] = 0.0
        spec["quantities"][0]["std_uncertainty"] = 0.0
        spec["results"] = [
            {"name": "fixed", "expression": "a + b"},
            {"name": "also_fixed", "expression": "a * b"},
        ]
        q, corr_in, budgets, result_cov, corr = run(spec)
        report = report_dict(q, corr_in, budgets, result_cov, corr)
        entry = report["result_correlations"]["fixed"]
        self.assertIsNone(entry["also_fixed"])
        self.assertEqual(budgets[0].std_uncertainty, 0.0)


def spec_quantities(spec):
    return load_quantities(spec["quantities"])


class TestCli(unittest.TestCase):
    def test_cli_json_and_csv(self):
        root = Path(__file__).resolve().parent.parent
        example = root / "examples" / "cancellation.json"
        out_json = root / "examples" / "_out.json"
        try:
            proc = subprocess.run(
                [sys.executable, "-m", "uncertainty_budget",
                 str(example), "--json", str(out_json)],
                cwd=root, capture_output=True, text=True)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            report = json.loads(out_json.read_text(encoding="utf-8"))
            names = [r["name"] for r in report["results"]]
            self.assertEqual(names, ["difference", "total", "ratio"])
            csv_path = Path(str(out_json) + ".budget.csv")
            self.assertTrue(csv_path.exists())
            csv_path.unlink()
        finally:
            out_json.unlink(missing_ok=True)

    def test_cli_rejects_bad_matrix(self):
        root = Path(__file__).resolve().parent.parent
        bad = root / "examples" / "_bad.json"
        bad.write_text(json.dumps(base_spec(
            correlation=[[1.0, 0.9, 0.9], [0.9, 1.0, 0.0], [0.9, 0.0, 1.0]]
        ) | {"quantities": base_spec()["quantities"] + [
            {"name": "c", "nominal": 1.0, "std_uncertainty": 0.1}]}),
            encoding="utf-8")
        try:
            proc = subprocess.run(
                [sys.executable, "-m", "uncertainty_budget", str(bad)],
                cwd=root, capture_output=True, text=True)
            self.assertEqual(proc.returncode, 2)
            self.assertIn("positive semidefinite", proc.stderr)
        finally:
            bad.unlink(missing_ok=True)


if __name__ == "__main__":
    unittest.main()

    def test_syntax_error_located(self):
        spec = base_spec()
        spec["results"] = [{"name": "s", "expression": "a +* b"}]
        with self.assertRaises(ExpressionError) as ctx:
            run(spec)
        self.assertIsNotNone(ctx.exception.position)

    def test_domain_errors(self):
        for expr in ("sqrt(-1)", "ln(0)", "1 / (a - a)", "sqrt(a - a)",
                     "(a - a)^-1"):
            spec = base_spec()
            spec["results"] = [{"name": "s", "expression": expr}]
            with self.assertRaises(ExpressionError, msg=expr):
                run(spec)

    def test_non_integer_exponent_rejected(self):
        spec = base_spec()
        spec["results"] = [{"name": "s", "expression": "a^0.5"}]
        with self.assertRaises(ExpressionError):
            run(spec)

    def test_no_arbitrary_code(self):
        spec = base_spec()
        spec["results"] = [
            {"name": "s", "expression": "__import__('os').system('echo hi')"}]
        with self.assertRaises(ExpressionError):
            run(spec)

    def test_singular_matrix_accepted(self):
        _, _, budgets, _, _ = run(
            base_spec(correlation=[[1.0, 1.0], [1.0, 1.0]]))
        self.assertAlmostEqual(budgets[0].variance, 1.0)

    def test_perfect_negative_correlation_cancels(self):
        spec = base_spec(correlation=[[1.0, -1.0], [-1.0, 1.0]])
        spec["results"] = [{"name": "d", "expression": "a - b"}]
        _, _, budgets, _, corr = run(spec)
        self.assertEqual(budgets[0].variance, 0.0)
        self.assertEqual(budgets[0].std_uncertainty, 0.0)
        self.assertIsNone(corr[0][0])

    def test_negative_std_uncertainty_rejected(self):
        spec = base_spec()
        spec["quantities"][0]["std_uncertainty"] = -0.1
        with self.assertRaises(InputError):
            run(spec)
