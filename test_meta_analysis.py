"""meta_analysis 与 report 的单元测试（标准库 unittest）。"""
from __future__ import annotations

import csv
import json
import math
import tempfile
import unittest
from pathlib import Path

from meta_analysis import (
    DataError,
    ExcludedStudy,
    Study,
    StudyEffect,
    compute_effect,
    load_studies,
    run_meta_analysis,
)
from report import to_dict, write_csv, write_json


def write_input(path: Path, rows: list[tuple]) -> None:
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            ["study_id", "exp_events", "exp_nonevents", "ctl_events", "ctl_nonevents"]
        )
        writer.writerows(rows)


class TestLoadStudies(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.path = Path(self.tmp.name) / "in.csv"

    def tearDown(self) -> None:
        self.tmp.cleanup()

    def test_valid_rows(self):
        write_input(self.path, [("S1", 5, 15, 8, 12)])
        studies = load_studies(self.path)
        self.assertEqual(studies, [Study("S1", 5, 15, 8, 12)])

    def test_duplicate_id_reports_row(self):
        write_input(self.path, [("S1", 1, 9, 2, 8), ("S1", 3, 7, 4, 6)])
        with self.assertRaisesRegex(DataError, "第 3 行.*重复"):
            load_studies(self.path)

    def test_negative_count_reports_row(self):
        write_input(self.path, [("S1", -1, 10, 2, 8)])
        with self.assertRaisesRegex(DataError, "第 2 行.*负数"):
            load_studies(self.path)

    def test_non_integer_count(self):
        write_input(self.path, [("S1", "a", 10, 2, 8)])
        with self.assertRaisesRegex(DataError, "第 2 行.*不是整数"):
            load_studies(self.path)

    def test_zero_group_total(self):
        write_input(self.path, [("S1", 0, 0, 2, 8)])
        with self.assertRaisesRegex(DataError, "实验组总人数必须为正"):
            load_studies(self.path)

    def test_missing_column(self):
        self.path.write_text("study_id,exp_events\nS1,1\n", encoding="utf-8")
        with self.assertRaisesRegex(DataError, "缺少必需列"):
            load_studies(self.path)


class TestComputeEffect(unittest.TestCase):
    def test_no_correction_when_no_zero_cell(self):
        effect = compute_effect(Study("S1", 10, 90, 20, 80))
        self.assertIsInstance(effect, StudyEffect)
        expected = math.log((10 * 80) / (90 * 20))
        self.assertAlmostEqual(effect.log_or, expected)
        self.assertAlmostEqual(effect.variance, 1 / 10 + 1 / 90 + 1 / 20 + 1 / 80)
        self.assertFalse(effect.corrected)

    def test_zero_cell_triggers_half_correction(self):
        effect = compute_effect(Study("S1", 0, 20, 5, 15))
        self.assertTrue(effect.corrected)
        expected = math.log((0.5 * 15.5) / (20.5 * 5.5))
        self.assertAlmostEqual(effect.log_or, expected)
        self.assertAlmostEqual(effect.variance, 1 / 0.5 + 1 / 20.5 + 1 / 5.5 + 1 / 15.5)

    def test_double_zero_excluded(self):
        outcome = compute_effect(Study("S1", 0, 20, 0, 30))
        self.assertIsInstance(outcome, ExcludedStudy)
        self.assertIn("均无事件", outcome.reason)

    def test_all_events_excluded(self):
        outcome = compute_effect(Study("S1", 20, 0, 30, 0))
        self.assertIsInstance(outcome, ExcludedStudy)
        self.assertIn("全部发生事件", outcome.reason)

    def test_ci_from_log_scale(self):
        effect = compute_effect(Study("S1", 10, 90, 20, 80))
        half = 1.959963984540054 * math.sqrt(effect.variance)
        self.assertAlmostEqual(effect.ci_lower, math.exp(effect.log_or - half))
        self.assertAlmostEqual(effect.ci_upper, math.exp(effect.log_or + half))


SAMPLE = [
    Study("A", 12, 88, 20, 80),
    Study("B", 8, 42, 15, 35),
    Study("C", 0, 30, 4, 26),
    Study("D", 25, 75, 25, 75),
]


class TestPooling(unittest.TestCase):
    def setUp(self):
        self.result = run_meta_analysis(SAMPLE)

    def test_fixed_effect_matches_manual_inverse_variance(self):
        effects = [compute_effect(s) for s in SAMPLE]
        weights = [1.0 / e.variance for e in effects]
        pooled = sum(w * e.log_or for w, e in zip(weights, effects)) / sum(weights)
        self.assertAlmostEqual(self.result.fixed.log_or, pooled)
        self.assertAlmostEqual(self.result.fixed.se, math.sqrt(1.0 / sum(weights)))

    def test_heterogeneity_quantities(self):
        h = self.result.random.heterogeneity
        self.assertTrue(h.estimable)
        self.assertEqual(h.df, 3)
        self.assertGreaterEqual(h.tau_squared, 0.0)
        self.assertGreaterEqual(h.i_squared, 0.0)
        self.assertLessEqual(h.i_squared, 100.0)
        # 手工复算 Q 与 tau^2
        effects = [compute_effect(s) for s in SAMPLE]
        weights = [1.0 / e.variance for e in effects]
        pooled = sum(w * e.log_or for w, e in zip(weights, effects)) / sum(weights)
        q = sum(w * (e.log_or - pooled) ** 2 for w, e in zip(weights, effects))
        c = sum(weights) - sum(w * w for w in weights) / sum(weights)
        self.assertAlmostEqual(h.q, q)
        self.assertAlmostEqual(h.tau_squared, max(0.0, (q - 3) / c))
        self.assertAlmostEqual(h.i_squared, max(0.0, (q - 3) / q) * 100.0)

    def test_random_weights_use_tau(self):
        tau2 = self.result.random.heterogeneity.tau_squared
        for s in self.result.studies:
            self.assertAlmostEqual(
                s.weight_random,
                (1.0 / (s.variance + tau2))
                / sum(1.0 / (x.variance + tau2) for x in self.result.studies)
                * 100.0,
            )
        self.assertAlmostEqual(sum(s.weight_fixed for s in self.result.studies), 100.0)
        self.assertAlmostEqual(sum(s.weight_random for s in self.result.studies), 100.0)

    def test_random_ci_wider_or_equal_when_tau_positive(self):
        if self.result.random.heterogeneity.tau_squared > 0:
            self.assertGreater(self.result.random.se, self.result.fixed.se)

    def test_zero_cell_study_flagged(self):
        flagged = {s.study_id: s.corrected for s in self.result.studies}
        self.assertEqual(flagged, {"A": False, "B": False, "C": True, "D": False})


class TestLeaveOneOut(unittest.TestCase):
    def setUp(self):
        self.result = run_meta_analysis(SAMPLE)

    def test_one_entry_per_study(self):
        self.assertEqual(
            [loo.omitted_id for loo in self.result.leave_one_out],
            ["A", "B", "C", "D"],
        )

    def test_loo_random_weights_recomputed(self):
        # 移除 C 后，随机效应合并值应等于仅对剩余研究重新估计 tau^2 的结果
        loo = next(x for x in self.result.leave_one_out if x.omitted_id == "C")
        remaining = [s for s in SAMPLE if s.study_id != "C"]
        reference = run_meta_analysis(remaining)
        self.assertAlmostEqual(loo.random.log_or, reference.random.log_or)
        self.assertAlmostEqual(
            loo.random.heterogeneity.tau_squared,
            reference.random.heterogeneity.tau_squared,
        )

    def test_loo_differs_from_full_when_influential(self):
        loo = next(x for x in self.result.leave_one_out if x.omitted_id == "C")
        self.assertNotAlmostEqual(loo.random.log_or, self.result.random.log_or)


class TestEdgeCases(unittest.TestCase):
    def test_single_study_heterogeneity_not_estimable(self):
        result = run_meta_analysis([Study("A", 5, 15, 10, 10)])
        self.assertFalse(result.random.heterogeneity.estimable)
        self.assertIsNone(result.random.heterogeneity.q)
        self.assertAlmostEqual(result.random.log_or, result.fixed.log_or)
        # 留一后无剩余研究
        loo = result.leave_one_out[0]
        self.assertEqual(loo.n_remaining, 0)
        self.assertIsNone(loo.fixed)
        self.assertIsNone(loo.random)
        self.assertIn("无剩余研究", loo.note)

    def test_two_studies_loo_keeps_interval(self):
        result = run_meta_analysis(
            [Study("A", 5, 15, 10, 10), Study("B", 8, 12, 12, 8)]
        )
        for loo in result.leave_one_out:
            self.assertEqual(loo.n_remaining, 1)
            self.assertFalse(loo.random.heterogeneity.estimable)
            self.assertLess(loo.random.ci_lower, loo.random.odds_ratio)
            self.assertGreater(loo.random.ci_upper, loo.random.odds_ratio)

    def test_excluded_studies_recorded_not_counted(self):
        result = run_meta_analysis(
            SAMPLE + [Study("E", 0, 10, 0, 10), Study("F", 10, 0, 10, 0)]
        )
        self.assertEqual(len(result.studies), 4)
        self.assertEqual([e.study_id for e in result.excluded], ["E", "F"])
        self.assertIn("均无事件", result.excluded[0].reason)
        self.assertIn("全部发生事件", result.excluded[1].reason)

    def test_all_excluded_raises(self):
        with self.assertRaisesRegex(DataError, "没有可纳入分析的研究"):
            run_meta_analysis([Study("E", 0, 10, 0, 10)])


class TestReports(unittest.TestCase):
    def setUp(self):
        self.result = run_meta_analysis(SAMPLE + [Study("E", 0, 10, 0, 10)])
        self.tmp = tempfile.TemporaryDirectory()

    def tearDown(self):
        self.tmp.cleanup()

    def test_json_roundtrip(self):
        path = Path(self.tmp.name) / "out.json"
        write_json(self.result, path)
        data = json.loads(path.read_text(encoding="utf-8"))
        self.assertEqual(data["n_included"], 4)
        self.assertEqual(data["n_excluded"], 1)
        self.assertEqual(len(data["studies"]), 4)
        self.assertEqual(len(data["leave_one_out"]), 4)
        self.assertIn("tau_squared", data["pooled_random"]["heterogeneity"])

    def test_csv_report_sections(self):
        path = Path(self.tmp.name) / "out.csv"
        write_csv(self.result, path)
        with open(path, newline="", encoding="utf-8") as fh:
            rows = list(csv.reader(fh))
        sections = [r[0] for r in rows[1:]]
        self.assertEqual(sections.count("study"), 4)
        self.assertEqual(sections.count("excluded"), 1)
        self.assertEqual(sections.count("pooled"), 2)
        self.assertEqual(sections.count("leave_one_out"), 8)

    def test_to_dict_counts(self):
        data = to_dict(self.result)
        self.assertEqual(data["n_included"] + data["n_excluded"], 5)


if __name__ == "__main__":
    unittest.main()
