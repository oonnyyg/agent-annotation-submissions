import csv
import json
import os
import tempfile
import unittest

from gage_study.anova import compute_anova
from gage_study.data import (
    Config,
    GageStudyError,
    load_config,
    load_measurements,
)
from gage_study.report import write_all
from gage_study.variance import compute_variance_components


def make_data(rows):
    """rows: list of (part, operator, replicate, value); returns MeasurementData."""
    with tempfile.TemporaryDirectory() as tmp:
        csv_path = os.path.join(tmp, "m.csv")
        with open(csv_path, "w", newline="", encoding="utf-8") as fh:
            writer = csv.writer(fh)
            writer.writerow(["part", "operator", "replicate", "value"])
            writer.writerows(rows)
        return load_measurements(csv_path, Config())


def grid(parts, ops, reps, value_fn):
    return [
        (p, o, str(r), value_fn(p, o, r))
        for p in parts
        for o in ops
        for r in range(1, reps + 1)
    ]


class ValidationTests(unittest.TestCase):
    def test_missing_cell_rejected(self):
        rows = grid(["P1", "P2"], ["A", "B"], 2, lambda p, o, r: 1.0)
        rows = [r for r in rows if not (r[0] == "P2" and r[1] == "B")]
        with self.assertRaises(GageStudyError):
            make_data(rows)

    def test_duplicate_replicate_rejected(self):
        rows = grid(["P1", "P2"], ["A", "B"], 2, lambda p, o, r: 1.0)
        rows.append(("P1", "A", "1", 2.0))
        with self.assertRaises(GageStudyError):
            make_data(rows)

    def test_non_finite_rejected(self):
        rows = grid(["P1", "P2"], ["A", "B"], 2, lambda p, o, r: 1.0)
        rows[0] = ("P1", "A", "1", float("nan"))
        with self.assertRaises(GageStudyError):
            make_data(rows)

    def test_unbalanced_rejected(self):
        rows = grid(["P1", "P2"], ["A", "B"], 2, lambda p, o, r: 1.0)
        rows.append(("P1", "A", "3", 1.0))
        with self.assertRaises(GageStudyError):
            make_data(rows)

    def test_single_replicate_rejected(self):
        rows = grid(["P1", "P2"], ["A", "B"], 1, lambda p, o, r: 1.0)
        with self.assertRaises(GageStudyError):
            make_data(rows)

    def test_too_few_parts_or_operators(self):
        with self.assertRaises(GageStudyError):
            make_data(grid(["P1"], ["A", "B"], 2, lambda p, o, r: 1.0))
        with self.assertRaises(GageStudyError):
            make_data(grid(["P1", "P2"], ["A"], 2, lambda p, o, r: 1.0))


class AnovaTests(unittest.TestCase):
    def test_ss_decomposition(self):
        rows = grid(
            ["P1", "P2", "P3"],
            ["A", "B"],
            3,
            lambda p, o, r: hash((p, o, r)) % 17 / 3.0,
        )
        data = make_data(rows)
        anova = compute_anova(data)
        self.assertAlmostEqual(anova.ss_check_residual, 0.0, places=9)
        self.assertEqual(anova.total_df, sum(row.df for row in anova.rows))

    def test_pure_repeat_noise(self):
        noise = {"1": -0.5, "2": 0.5}
        rows = grid(
            ["P1", "P2", "P3"],
            ["A", "B"],
            2,
            lambda p, o, r: 10.0 + noise[str(r)],
        )
        data = make_data(rows)
        anova = compute_anova(data)
        vc = compute_variance_components(data, anova)
        self.assertAlmostEqual(vc.repeatability, 0.5)
        self.assertLessEqual(vc.part, 0.0)
        self.assertLessEqual(vc.operator, 0.0)
        self.assertLessEqual(vc.interaction, 0.0)
        self.assertAlmostEqual(vc.gage, 0.5)
        self.assertAlmostEqual(vc.total, 0.5)
        self.assertTrue(vc.warnings)

    def test_clear_interaction(self):
        def value(p, o, r):
            base = 10.0 if (p == "P1") == (o == "A") else 0.0
            return base + (0.05 if r == 1 else -0.05)

        rows = grid(["P1", "P2"], ["A", "B"], 2, value)
        data = make_data(rows)
        anova = compute_anova(data)
        vc = compute_variance_components(data, anova)
        self.assertGreater(vc.interaction, 1.0)
        # with a=b=2 the huge interaction MS pushes part/operator estimates
        # negative; the raw negative values must be kept and warned about
        self.assertLess(vc.part, 0.0)
        self.assertLess(vc.operator, 0.0)
        self.assertTrue(any("part" in w for w in vc.warnings))
        self.assertAlmostEqual(vc.repeatability, 0.005, places=9)

    def test_all_constant(self):
        rows = grid(["P1", "P2"], ["A", "B"], 3, lambda p, o, r: 7.5)
        data = make_data(rows)
        anova = compute_anova(data)
        vc = compute_variance_components(data, anova)
        self.assertEqual(vc.total, 0.0)
        self.assertIsNone(vc.contribution_pct["gage"])
        self.assertIsNone(vc.stddev_pct["gage"])

    def test_order_independence(self):
        def value(p, o, r):
            return (hash((p, o, r)) % 100) / 10.0

        rows = grid(["P1", "P2", "P3"], ["A", "B"], 3, value)
        shuffled = list(reversed(rows))
        d1, d2 = make_data(rows), make_data(shuffled)
        a1, a2 = compute_anova(d1), compute_anova(d2)
        v1 = compute_variance_components(d1, a1)
        v2 = compute_variance_components(d2, a2)
        for r1, r2 in zip(a1.rows, a2.rows):
            self.assertAlmostEqual(r1.ss, r2.ss)
        self.assertAlmostEqual(v1.total, v2.total)
        self.assertEqual(a1.cell_means, a2.cell_means)
