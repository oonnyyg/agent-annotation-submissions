import csv
import json
import math
import os
import tempfile
import unittest

from raking.core import (STATUS_CONVERGED, STATUS_NOT_CONVERGED,
                         STATUS_UNREACHABLE, effective_sample_size, rake)
from raking.errors import RakingError
from raking.inputs import Margin, Sample, load_config, load_samples
from raking.report import (DIAGNOSTICS_JSON, MARGINS_CSV, WEIGHTS_CSV,
                           write_diagnostics, write_success)


def make_samples(specs):
    """specs: list of (id, fields-dict, weight)."""
    return [Sample(sid, dict(fields), w) for sid, fields, w in specs]


class CoreRakingTests(unittest.TestCase):
    def test_single_margin_converges(self):
        samples = make_samples([
            ("a", {"sex": "M"}, 1.0),
            ("b", {"sex": "M"}, 1.0),
            ("c", {"sex": "F"}, 1.0),
        ])
        margins = [Margin("sex", ("sex",), {"M": 100.0, "F": 100.0})]
        result = rake(samples, margins, 1e-12, 100)
        self.assertEqual(result.status, STATUS_CONVERGED)
        self.assertAlmostEqual(result.achieved["sex"]["M"], 100.0)
        self.assertAlmostEqual(result.achieved["sex"]["F"], 100.0)

    def test_joint_margin_converges(self):
        samples = make_samples([
            ("a", {"age": "Y", "sex": "M"}, 1.0),
            ("b", {"age": "Y", "sex": "F"}, 2.0),
            ("c", {"age": "O", "sex": "M"}, 3.0),
            ("d", {"age": "O", "sex": "F"}, 4.0),
        ])
        margins = [
            Margin("age", ("age",), {"Y": 60.0, "O": 40.0}),
            Margin("sex", ("sex",), {"M": 55.0, "F": 45.0}),
            Margin("age_sex", ("age", "sex"),
                   {"Y|M": 30.0, "Y|F": 30.0, "O|M": 25.0, "O|F": 15.0}),
        ]
        result = rake(samples, margins, 1e-9, 500)
        self.assertEqual(result.status, STATUS_CONVERGED)
        for name, targets in (("age", margins[0].targets),
                              ("sex", margins[1].targets),
                              ("age_sex", margins[2].targets)):
            for key, target in targets.items():
                self.assertAlmostEqual(result.achieved[name][key], target,
                                       places=6)

    def test_zero_target_zeroes_class(self):
        samples = make_samples([
            ("a", {"g": "x"}, 1.0),
            ("b", {"g": "y"}, 1.0),
        ])
        margins = [Margin("g", ("g",), {"x": 10.0, "y": 0.0})]
        result = rake(samples, margins, 1e-12, 50)
        self.assertEqual(result.status, STATUS_CONVERGED)
        self.assertEqual(result.weights[1], 0.0)
        self.assertAlmostEqual(result.weights[0], 10.0)

    def test_positive_target_without_support_is_unreachable(self):
        samples = make_samples([("a", {"g": "x"}, 1.0)])
        margins = [Margin("g", ("g",), {"x": 5.0, "z": 5.0})]
        result = rake(samples, margins, 1e-12, 50)
        self.assertEqual(result.status, STATUS_UNREACHABLE)
        self.assertEqual(result.failures[0].category, "z")
        self.assertEqual(result.failures[0].iteration, 0)

    def test_conflicting_zero_targets_unreachable(self):
        samples = make_samples([
            ("a", {"age": "Y", "sex": "M"}, 1.0),
            ("b", {"age": "Y", "sex": "F"}, 1.0),
        ])
        margins = [
            Margin("sex", ("sex",), {"M": 0.0, "F": 10.0}),
            Margin("joint", ("age", "sex"), {"Y|M": 4.0, "Y|F": 6.0}),
        ]
        result = rake(samples, margins, 1e-12, 50)
        self.assertEqual(result.status, STATUS_UNREACHABLE)
        self.assertIn("zero", result.failures[0].reason)

    def test_max_iterations_marks_not_converged(self):
        samples = make_samples([
            ("a", {"g": "x", "h": "p"}, 1.0),
            ("b", {"g": "x", "h": "q"}, 1.0),
            ("c", {"g": "y", "h": "q"}, 1.0),
        ])
        margins = [
            Margin("g", ("g",), {"x": 1.5, "y": 1.5}),
            Margin("h", ("h",), {"p": 1.0, "q": 2.0}),
        ]
        result = rake(samples, margins, 1e-300, 3)
        self.assertEqual(result.status, STATUS_NOT_CONVERGED)
        self.assertEqual(result.iterations, 3)
        self.assertEqual(len(result.history), 3)
        self.assertFalse(result.converged)

    def test_history_records_worst_margin(self):
        samples = make_samples([
            ("a", {"g": "x"}, 1.0),
            ("b", {"g": "y"}, 1.0),
        ])
        margins = [Margin("g", ("g",), {"x": 3.0, "y": 3.0})]
        result = rake(samples, margins, 1e-12, 10)
        self.assertTrue(result.history)
        first = result.history[0]
        self.assertEqual(first.margin, "g")
        self.assertIn(first.category, ("x", "y"))

    def test_effective_sample_size(self):
        self.assertAlmostEqual(effective_sample_size([1.0, 1.0, 1.0, 1.0]), 4.0)
        self.assertAlmostEqual(effective_sample_size([2.0, 1.0]), 9.0 / 5.0)
        self.assertEqual(effective_sample_size([0.0, 0.0]), 0.0)


SAMPLE_CSV = "id,weight,age,sex\n" + "\n".join([
    "s1,1.0,Y,M",
    "s2,2.0,Y,F",
    "s3,3.0,O,M",
    "s4,4.0,O,F",
]) + "\n"

TARGETS = {
    "tolerance": 1e-9,
    "max_iterations": 500,
    "margins": [
        {"name": "age", "fields": ["age"],
         "targets": {"Y": 60, "O": 40}},
        {"name": "sex", "fields": ["sex"],
         "targets": {"M": 55, "F": 45}},
        {"name": "age_sex", "fields": ["age", "sex"],
         "targets": {"Y|M": 30, "Y|F": 30, "O|M": 25, "O|F": 15}},
    ],
}


class WorkspaceMixin:
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.samples_path = os.path.join(self.tmp.name, "samples.csv")
        with open(self.samples_path, "w", encoding="utf-8") as fh:
            fh.write(SAMPLE_CSV)

    def write_targets(self, payload):
        path = os.path.join(self.tmp.name, "targets.json")
        if isinstance(payload, str):
            with open(path, "w", encoding="utf-8") as fh:
                fh.write(payload)
        else:
            with open(path, "w", encoding="utf-8") as fh:
                json.dump(payload, fh)
        return path


class InputValidationTests(WorkspaceMixin, unittest.TestCase):
    def load(self, payload):
        samples, fields = load_samples(self.samples_path)
        return load_config(self.write_targets(payload), samples, fields)

    def test_valid_config_loads(self):
        config = self.load(TARGETS)
        self.assertEqual(len(config.margins), 3)
        self.assertEqual(config.margins[2].fields, ("age", "sex"))

    def test_duplicate_json_key_rejected(self):
        text = ('{"margins": [{"name": "m", "fields": ["age"], '
                '"targets": {"Y": 50, "Y": 50, "O": 50}}]}')
        with self.assertRaisesRegex(RakingError, "duplicate key"):
            self.load(text)

    def test_missing_sample_category_rejected(self):
        payload = {"margins": [{"name": "m", "fields": ["age"],
                                "targets": {"Y": 100}}]}
        with self.assertRaisesRegex(RakingError, "no target"):
            self.load(payload)

    def test_unequal_margin_totals_rejected(self):
        payload = {"margins": [
            {"name": "a", "fields": ["age"], "targets": {"Y": 60, "O": 40}},
            {"name": "s", "fields": ["sex"], "targets": {"M": 50, "F": 40}},
        ]}
        with self.assertRaisesRegex(RakingError, "total"):
            self.load(payload)

    def test_negative_target_rejected(self):
        payload = {"margins": [{"name": "a", "fields": ["age"],
                                "targets": {"Y": 110, "O": -10}}]}
        with self.assertRaisesRegex(RakingError, "non-negative"):
            self.load(payload)

    def test_unknown_field_rejected(self):
        payload = {"margins": [{"name": "z", "fields": ["income"],
                                "targets": {"hi": 100}}]}
        with self.assertRaisesRegex(RakingError, "unknown field"):
            self.load(payload)

    def test_duplicate_sample_id_rejected(self):
        path = os.path.join(self.tmp.name, "dup.csv")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("id,weight,age\ns1,1,Y\ns1,2,O\n")
        with self.assertRaisesRegex(RakingError, "duplicate sample id"):
            load_samples(path)

    def test_nonpositive_weight_rejected(self):
        path = os.path.join(self.tmp.name, "bad.csv")
        with open(path, "w", encoding="utf-8") as fh:
            fh.write("id,weight,age\ns1,0,Y\n")
        with self.assertRaisesRegex(RakingError, "positive"):
            load_samples(path)


class CliTests(WorkspaceMixin, unittest.TestCase):
    def run_cli(self, targets_payload, out_dir):
        from raking.cli import main
        targets_path = self.write_targets(targets_payload)
        return main([self.samples_path, targets_path, "--out-dir", out_dir])

    def test_success_outputs(self):
        out_dir = os.path.join(self.tmp.name, "out")
        code = self.run_cli(TARGETS, out_dir)
        self.assertEqual(code, 0)
        with open(os.path.join(out_dir, WEIGHTS_CSV), encoding="utf-8") as fh:
            rows = list(csv.DictReader(fh))
        self.assertEqual(len(rows), 4)
        for row in rows:
            final = float(row["final_weight"])
            multiplier = float(row["multiplier"])
            self.assertAlmostEqual(
                final, float(row["initial_weight"]) * multiplier)
        with open(os.path.join(out_dir, MARGINS_CSV), encoding="utf-8") as fh:
            text = fh.read()
        self.assertIn("effective_sample_size", text)
        self.assertIn("age_sex", text)
        self.assertFalse(os.path.exists(os.path.join(out_dir,
                                                     DIAGNOSTICS_JSON)))

    def test_failure_writes_only_diagnostics(self):
        payload = {"max_iterations": 50, "margins": [
            {"name": "sex", "fields": ["sex"],
             "targets": {"M": 0, "F": 100}},
            {"name": "joint", "fields": ["age", "sex"],
             "targets": {"Y|M": 25, "Y|F": 25, "O|M": 25, "O|F": 25}},
        ]}
        out_dir = os.path.join(self.tmp.name, "out")
        code = self.run_cli(payload, out_dir)
        self.assertEqual(code, 1)
        self.assertFalse(os.path.exists(os.path.join(out_dir, WEIGHTS_CSV)))
        diag_path = os.path.join(out_dir, DIAGNOSTICS_JSON)
        with open(diag_path, encoding="utf-8") as fh:
            diag = json.load(fh)
        self.assertEqual(diag["status"], STATUS_UNREACHABLE)
        self.assertFalse(diag["converged"])
        self.assertTrue(diag["failures"])

    def test_failure_does_not_overwrite_success(self):
        out_dir = os.path.join(self.tmp.name, "out")
        self.assertEqual(self.run_cli(TARGETS, out_dir), 0)
        weights_path = os.path.join(out_dir, WEIGHTS_CSV)
        with open(weights_path, encoding="utf-8") as fh:
            good = fh.read()
        bad_payload = {"margins": [
            {"name": "sex", "fields": ["sex"],
             "targets": {"M": 0, "F": 100}},
            {"name": "joint", "fields": ["age", "sex"],
             "targets": {"Y|M": 25, "Y|F": 25, "O|M": 25, "O|F": 25}},
        ]}
        self.assertEqual(self.run_cli(bad_payload, out_dir), 1)
        with open(weights_path, encoding="utf-8") as fh:
            self.assertEqual(fh.read(), good)

    def test_input_error_exit_code(self):
        from raking.cli import main
        bad = os.path.join(self.tmp.name, "bad.json")
        with open(bad, "w", encoding="utf-8") as fh:
            fh.write("{not json")
        code = main([self.samples_path, bad, "--out-dir",
                     os.path.join(self.tmp.name, "out")])
        self.assertEqual(code, 2)


if __name__ == "__main__":
    unittest.main()
