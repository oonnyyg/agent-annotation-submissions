import math
import unittest

from binary_meta.data import Study
from binary_meta.stats import (
    ExcludedStudy,
    StudyEffect,
    compute_effect,
    leave_one_out,
    meta_analyze,
)


def effect(a, b, c, d, sid="s"):
    return compute_effect(Study(sid, a, b, c, d))


class ComputeEffectTest(unittest.TestCase):
    def test_no_zero_cells_no_correction(self):
        e = effect(10, 90, 20, 80)
        self.assertIsInstance(e, StudyEffect)
        self.assertFalse(e.corrected)
        self.assertAlmostEqual(e.log_or, math.log((10 * 80) / (90 * 20)))
        self.assertAlmostEqual(e.variance, 1 / 10 + 1 / 90 + 1 / 20 + 1 / 80)

    def test_single_zero_cell_triggers_correction(self):
        e = effect(0, 50, 3, 47)
        self.assertTrue(e.corrected)
        self.assertAlmostEqual(e.log_or, math.log((0.5 * 47.5) / (50.5 * 3.5)))
        self.assertAlmostEqual(e.variance, 1 / 0.5 + 1 / 50.5 + 1 / 3.5 + 1 / 47.5)

    def test_double_zero_events_excluded(self):
        e = effect(0, 50, 0, 60)
        self.assertIsInstance(e, ExcludedStudy)
        self.assertIn("no events", e.reason)

    def test_all_events_excluded(self):
        e = effect(50, 0, 60, 0)
        self.assertIsInstance(e, ExcludedStudy)
        self.assertIn("all subjects", e.reason)

    def test_zero_nonevents_one_group_corrected_not_excluded(self):
        e = effect(50, 0, 40, 10)
        self.assertIsInstance(e, StudyEffect)
        self.assertTrue(e.corrected)


class MetaAnalyzeTest(unittest.TestCase):
    def setUp(self):
        self.effects = [
            effect(12, 88, 20, 80, "a"),
            effect(8, 42, 15, 35, "b"),
            effect(30, 70, 25, 75, "c"),
        ]

    def test_fixed_effect_inverse_variance(self):
        meta = meta_analyze(self.effects)
        ws = [1 / e.variance for e in self.effects]
        expected = sum(w * e.log_or for w, e in zip(ws, self.effects)) / sum(ws)
        self.assertAlmostEqual(meta.fixed.log_or, expected)
        self.assertAlmostEqual(meta.fixed.se, math.sqrt(1 / sum(ws)))
        self.assertAlmostEqual(sum(meta.fixed_weights), 100.0)
        self.assertAlmostEqual(sum(meta.random_weights), 100.0)

    def test_homogeneous_tau2_zero_and_i2_zero(self):
        similar = [effect(10, 90, 10, 90, "x"), effect(12, 108, 11, 99, "y")]
        meta = meta_analyze(similar)
        self.assertEqual(meta.heterogeneity.tau2, 0.0)
        self.assertGreaterEqual(meta.heterogeneity.i2, 0.0)
        self.assertAlmostEqual(meta.random.log_or, meta.fixed.log_or, places=6)

    def test_heterogeneous_positive_tau2(self):
        diverse = [
            effect(40, 10, 10, 40, "p"),
            effect(10, 40, 40, 10, "q"),
            effect(35, 15, 12, 38, "r"),
        ]
        meta = meta_analyze(diverse)
        self.assertGreater(meta.heterogeneity.q, 2)
        self.assertGreater(meta.heterogeneity.tau2, 0.0)
        self.assertGreater(meta.heterogeneity.i2, 0.0)
        self.assertGreater(meta.random.se, 0.0)

    def test_single_study_heterogeneity_not_estimable(self):
        meta = meta_analyze([self.effects[0]])
        self.assertIsNone(meta.heterogeneity.q)
        self.assertIsNone(meta.heterogeneity.tau2)
        self.assertIsNotNone(meta.fixed)
        lo, hi = meta.fixed.ci_lo, meta.fixed.ci_hi
        self.assertLess(lo, meta.fixed.log_or)
        self.assertGreater(hi, meta.fixed.log_or)

    def test_empty_no_result(self):
        meta = meta_analyze([])
        self.assertIsNone(meta.fixed)
        self.assertIsNone(meta.random)
        self.assertEqual(meta.k, 0)

    def test_or_interval_is_exp_of_log_interval(self):
        meta = meta_analyze(self.effects)
        orr, lo, hi = meta.fixed.or_ci
        self.assertAlmostEqual(orr, math.exp(meta.fixed.log_or))
        self.assertAlmostEqual(lo, math.exp(meta.fixed.ci_lo))
        self.assertAlmostEqual(hi, math.exp(meta.fixed.ci_hi))


class LeaveOneOutTest(unittest.TestCase):
    def test_recomputes_each_omission(self):
        effects = [
            effect(12, 88, 20, 80, "a"),
            effect(8, 42, 15, 35, "b"),
            effect(30, 70, 25, 75, "c"),
        ]
        loo = leave_one_out(effects)
        self.assertEqual([sid for sid, _ in loo], ["a", "b", "c"])
        for sid, result in loo:
            self.assertEqual(result.k, 2)
            remaining = [e for e in effects if e.study_id != sid]
            direct = meta_analyze(remaining)
            self.assertAlmostEqual(result.random.log_or, direct.random.log_or)
            self.assertAlmostEqual(result.heterogeneity.tau2, direct.heterogeneity.tau2)

    def test_down_to_one_study_still_has_interval(self):
        effects = [effect(12, 88, 20, 80, "a"), effect(8, 42, 15, 35, "b")]
        loo = leave_one_out(effects)
        for _, result in loo:
            self.assertEqual(result.k, 1)
            self.assertIsNotNone(result.fixed)
            self.assertIsNone(result.heterogeneity.q)

    def test_single_study_leaves_nothing(self):
        loo = leave_one_out([effect(12, 88, 20, 80, "a")])
        _, result = loo[0]
        self.assertEqual(result.k, 0)
        self.assertIsNone(result.fixed)


if __name__ == "__main__":
    unittest.main()
