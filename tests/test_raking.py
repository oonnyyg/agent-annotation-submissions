import json
import math
import unittest

from raking.core import Margin, RakingError, load_samples, load_targets, rake


def make_rows():
    rows = []
    combos = [
        ("young", "F"), ("young", "M"), ("old", "F"),
        ("old", "M"), ("young", "F"), ("old", "M"),
    ]
    for i, (age, sex) in enumerate(combos):
        rows.append({"id": f"s{i}", "age": age, "sex": sex})
    return rows


class RakeConvergenceTests(unittest.TestCase):
    def test_single_margin_converges(self):
        rows = make_rows()
        margins = [Margin("age", ("age",), {("young",): 150.0, ("old",): 150.0})]
        result = rake(rows, [1.0] * 6, margins)
        self.assertTrue(result.converged)
        self.assertAlmostEqual(result.achieved["age"][("young",)], 150.0)
        self.assertAlmostEqual(result.achieved["age"][("old",)], 150.0)
        self.assertIsNotNone(result.effective_sample_size)

    def test_joint_margin_converges(self):
        rows = make_rows()
        targets = {
            ("young", "F"): 100.0,
            ("young", "M"): 50.0,
            ("old", "F"): 60.0,
            ("old", "M"): 90.0,
        }
        margins = [Margin("age_x_sex", ("age", "sex"), targets)]
        result = rake(rows, [1.0] * 6, margins)
        self.assertTrue(result.converged)
        for category, target in targets.items():
            self.assertAlmostEqual(result.achieved["age_x_sex"][category], target)

    def test_two_margins_both_checked_each_round(self):
        rows = make_rows()
        margins = [
            Margin("age", ("age",), {("young",): 180.0, ("old",): 120.0}),
            Margin("sex", ("sex",), {("F",): 160.0, ("M",): 140.0}),
        ]
        result = rake(rows, [1.0] * 6, margins)
        self.assertTrue(result.converged)
        for margin in margins:
            for category, target in margin.targets.items():
                self.assertAlmostEqual(result.achieved[margin.name][category], target, places=4)

    def test_zero_target_zeroes_category(self):
        rows = make_rows()
        margins = [Margin("age", ("age",), {("young",): 0.0, ("old",): 300.0})]
        result = rake(rows, [1.0] * 6, margins)
        self.assertTrue(result.converged)
        for row, weight in zip(rows, result.weights):
            if row["age"] == "young":
                self.assertEqual(weight, 0.0)
        self.assertAlmostEqual(result.achieved["age"][("old",)], 300.0)

    def test_unreachable_positive_target(self):
        rows = make_rows()
        joint = Margin(
            "age_x_sex",
            ("age", "sex"),
            {
                ("young", "F"): 0.0,
                ("old", "F"): 0.0,
                ("young", "M"): 150.0,
                ("old", "M"): 150.0,
            },
        )
        margins = [joint, Margin("sex", ("sex",), {("F",): 100.0, ("M",): 200.0})]
        result = rake(rows, [1.0] * 6, margins)
        self.assertFalse(result.converged)
        self.assertIn("不可达", result.failure_reason)
        self.assertIsNone(result.effective_sample_size)

    def test_max_rounds_marks_not_converged(self):
        rows = make_rows()
        margins = [
            Margin("age", ("age",), {("young",): 150.0, ("old",): 150.0}),
            Margin("sex", ("sex",), {("F",): 160.0, ("M",): 140.0}),
        ]
        result = rake(rows, [1.0] * 6, margins, max_rounds=1)
        self.assertFalse(result.converged)
        self.assertIn("最大轮数", result.failure_reason)
        self.assertEqual(len(result.rounds), 1)
        self.assertGreater(result.rounds[0].max_residual, 1e-6)
        full = rake(rows, [1.0] * 6, margins, max_rounds=200)
        self.assertTrue(full.converged)


class ValidationTests(unittest.TestCase):
    def test_missing_sample_category_rejected(self):
        rows = make_rows()
        margins = [Margin("age", ("age",), {("young",): 150.0})]
        with self.assertRaises(RakingError):
            rake(rows, [1.0] * 6, margins)

    def test_load_samples_rejects_duplicate_ids_and_bad_weights(self):
        import tempfile, os
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "s.csv")
            with open(path, "w", encoding="utf-8") as fh:
                fh.write("id,weight,age\na,1,young\na,2,old\n")
            with self.assertRaises(RakingError):
                load_samples(path, "id", "weight")
            with open(path, "w", encoding="utf-8") as fh:
                fh.write("id,weight,age\na,0,young\n")
            with self.assertRaises(RakingError):
                load_samples(path, "id", "weight")
            with open(path, "w", encoding="utf-8") as fh:
                fh.write("id,weight,age\na,1.5,young\n")
            ids, rows, weights = load_samples(path, "id", "weight")
            self.assertEqual(ids, ["a"])
            self.assertEqual(weights, [1.5])

    def test_load_targets_rejects_inconsistent_totals(self):
        import tempfile, os
        payload = {
            "margins": [
                {"name": "age", "fields": ["age"],
                 "targets": {"young": 100, "old": 100}},
                {"name": "sex", "fields": ["sex"],
                 "targets": {"F": 50, "M": 50}},
            ]
        }
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "t.json")
            with open(path, "w", encoding="utf-8") as fh:
                json.dump(payload, fh)
            with self.assertRaises(RakingError):
                load_targets(path)

    def test_load_targets_rejects_negative_and_duplicate(self):
        import tempfile, os
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "t.json")
            with open(path, "w", encoding="utf-8") as fh:
                json.dump({"margins": [
                    {"name": "age", "fields": ["age"],
                     "targets": {"young": -1, "old": 101}}
                ]}, fh)
            with self.assertRaises(RakingError):
                load_targets(path)

    def test_load_targets_joint_keys(self):
        import tempfile, os
        payload = {"margins": [
            {"name": "age_x_sex", "fields": ["age", "sex"],
             "targets": {"young||F": 25, "young||M": 25, "old||F": 25, "old||M": 25}}
        ]}
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "t.json")
            with open(path, "w", encoding="utf-8") as fh:
                json.dump(payload, fh)
            margins = load_targets(path)
            self.assertEqual(margins[0].fields, ("age", "sex"))
            self.assertEqual(margins[0].targets[("young", "F")], 25.0)


class CliTests(unittest.TestCase):
    def _write_case(self, tmp, targets):
        import csv, os
        samples = os.path.join(tmp, "samples.csv")
        with open(samples, "w", newline="", encoding="utf-8") as fh:
            writer = csv.writer(fh)
            writer.writerow(["id", "weight", "age", "sex"])
            for row in make_rows():
                writer.writerow([row["id"], "1.0", row["age"], row["sex"]])
        target_path = os.path.join(tmp, "targets.json")
        with open(target_path, "w", encoding="utf-8") as fh:
            json.dump({"margins": targets}, fh)
        return samples, target_path

    def test_cli_success_outputs(self):
        import csv, os, tempfile
        from raking.cli import main
        targets = [
            {"name": "age", "fields": ["age"],
             "targets": {"young": 180, "old": 120}},
            {"name": "sex", "fields": ["sex"],
             "targets": {"F": 160, "M": 140}},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            samples, target_path = self._write_case(tmp, targets)
            out = os.path.join(tmp, "out")
            code = main(["--samples", samples, "--targets", target_path, "--out", out])
            self.assertEqual(code, 0)
            with open(os.path.join(out, "weights.csv"), encoding="utf-8") as fh:
                rows = list(csv.DictReader(fh))
            self.assertEqual(len(rows), 6)
            for row in rows:
                self.assertAlmostEqual(
                    float(row["final_weight"]),
                    float(row["initial_weight"]) * float(row["multiplier"]),
                )
            with open(os.path.join(out, "margins.json"), encoding="utf-8") as fh:
                payload = json.load(fh)
            self.assertEqual(payload["status"], "converged")
            self.assertGreater(payload["effective_sample_size"], 0)
            for margin in payload["margins"]:
                for cat in margin["categories"]:
                    self.assertAlmostEqual(cat["target"], cat["achieved"], places=4)

    def test_cli_failure_keeps_success_outputs(self):
        import os, tempfile
        from raking.cli import main
        good = [
            {"name": "age", "fields": ["age"],
             "targets": {"young": 150, "old": 150}},
        ]
        bad = [
            {"name": "age_x_sex", "fields": ["age", "sex"],
             "targets": {"young||F": 0, "old||F": 0, "young||M": 150, "old||M": 150}},
            {"name": "sex", "fields": ["sex"],
             "targets": {"F": 100, "M": 200}},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            samples, good_path = self._write_case(tmp, good)
            out = os.path.join(tmp, "out")
            self.assertEqual(main(["--samples", samples, "--targets", good_path,
                                   "--out", out]), 0)
            weights_file = os.path.join(out, "weights.csv")
            before = os.path.getmtime(weights_file)
            _, bad_path = self._write_case(tmp, bad)
            code = main(["--samples", samples, "--targets", bad_path, "--out", out])
            self.assertEqual(code, 1)
            self.assertEqual(os.path.getmtime(weights_file), before)
            with open(os.path.join(out, "diagnostics.json"), encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertEqual(diag["status"], "failed")
            self.assertIn("不可达", diag["failure_reason"])
            self.assertEqual(len(diag["margins"]), 2)


if __name__ == "__main__":
    unittest.main()
