"""Tests for the BET fitting tool (stdlib unittest)."""

from __future__ import annotations

import json
import math
import tempfile
import unittest
from pathlib import Path

from bet_fit.data import Config, DataError, load_config, load_samples
from bet_fit.fit import AVOGADRO, analyze_sample, bet_transform, fit_interval
from bet_fit.report import render_report

SIGMA = 0.162  # N2 at 77 K, nm^2


def bet_isotherm(p, n_m, c):
    """Ideal BET isotherm: q in mol/g for relative pressure p."""
    return n_m * c * p / ((1.0 - p) * (1.0 - p + c * p))


def make_sample(n_m=3.0e-3, c=120.0, n=12, p_lo=0.01, p_hi=0.35):
    ps = [p_lo + (p_hi - p_lo) * i / (n - 1) for i in range(n)]
    qs = [bet_isotherm(p, n_m, c) for p in ps]
    return ps, qs


class SyntheticIsothermTest(unittest.TestCase):
    def test_recovers_known_area(self):
        n_m, c = 3.0e-3, 120.0
        ps, qs = make_sample(n_m, c)
        selected, candidates = analyze_sample(ps, qs, SIGMA)
        self.assertIsNotNone(selected)
        self.assertTrue(selected.accepted)
        self.assertAlmostEqual(selected.monolayer_mol_per_g, n_m, delta=1e-9)
        self.assertAlmostEqual(selected.bet_constant, c, delta=1e-6)
        expected_area = n_m * AVOGADRO * SIGMA * 1e-18
        self.assertAlmostEqual(selected.area_m2_per_g, expected_area,
                               delta=1e-6)
        self.assertAlmostEqual(selected.r_squared, 1.0, places=12)
        self.assertEqual(len(selected.residuals), selected.n_points)
        self.assertGreater(len(candidates), 1)

    def test_tie_break_prefers_more_points(self):
        ps, qs = make_sample()
        selected, _ = analyze_sample(ps, qs, SIGMA)
        # Perfectly linear data: all intervals tie on R^2 = 1, so the
        # longest interval (all points) must win.
        self.assertEqual(selected.n_points, len(ps))
        self.assertEqual(selected.start, 0)

    def test_manual_interval_is_screened(self):
        ps, qs = make_sample()
        selected, candidates = analyze_sample(ps, qs, SIGMA,
                                              interval=(0.05, 0.30))
        self.assertIsNotNone(selected)
        self.assertEqual(len(candidates), 1)
        self.assertGreaterEqual(selected.p_start, 0.05 - 1e-12)
        self.assertLessEqual(selected.p_end, 0.30 + 1e-12)


class ScreeningTest(unittest.TestCase):
    def test_rouquerol_violation_rejected(self):
        # q*(1-p) strictly decreasing -> every interval rejected.
        ps = [0.05, 0.10, 0.15, 0.20, 0.25, 0.30]
        qs = [0.02 / (1.0 - p) * (1.0 - 0.5 * p) for p in ps]
        rouq = [q * (1 - p) for p, q in zip(ps, qs)]
        self.assertTrue(all(b < a for a, b in zip(rouq, rouq[1:])))
        selected, candidates = analyze_sample(ps, qs, SIGMA)
        self.assertIsNone(selected)
        self.assertTrue(candidates)
        for c in candidates:
            self.assertIn("q*(1-p) decreases over interval", c.rejections)

    def test_negative_intercept_rejected(self):
        # Decreasing BET ordinate -> non-positive intercept.
        ps = [0.05, 0.10, 0.15, 0.20]
        ys = [0.05, 0.03, 0.02, 0.015]
        qs = [p / (y * (1 - p)) for p, y in zip(ps, ys)]
        res = fit_interval(ps, qs, 0, 3, SIGMA)
        self.assertFalse(res.accepted)
        self.assertTrue(any("intercept" in r or "constant" in r
                            for r in res.rejections))

    def test_undefined_r_squared_excluded(self):
        # Constant BET ordinate -> ss_tot = 0 -> R^2 undefined.
        ps = [0.05, 0.10, 0.15, 0.20]
        qs = [p / (0.1 * (1 - p)) for p in ps]
        res = fit_interval(ps, qs, 0, 3, SIGMA)
        self.assertIsNone(res.r_squared)
        selected, _ = analyze_sample(ps, qs, SIGMA)
        self.assertIsNone(selected)


class DataValidationTest(unittest.TestCase):
    def _write_csv(self, rows):
        tmp = tempfile.NamedTemporaryFile(
            "w", suffix=".csv", delete=False, encoding="utf-8")
        tmp.write("sample,relative_pressure,quantity_mol_per_g\n")
        tmp.writelines(rows)
        tmp.close()
        return tmp.name

    def test_duplicate_pressure_located(self):
        path = self._write_csv(
            ["s1,0.10,0.001\n", "s1,0.20,0.002\n", "s1,0.10,0.003\n"])
        with self.assertRaises(DataError) as ctx:
            load_samples(path)
        self.assertIn("duplicate", str(ctx.exception))
        self.assertIn("lines 2 and 4", str(ctx.exception))

    def test_non_finite_value_located(self):
        path = self._write_csv(["s1,0.10,0.001\n", "s1,nan,0.002\n"])
        with self.assertRaises(DataError) as ctx:
            load_samples(path)
        self.assertIn("line 3", str(ctx.exception))

    def test_pressure_out_of_range(self):
        for bad in ("0.0", "1.0", "1.5", "-0.2"):
            path = self._write_csv([f"s1,{bad},0.001\n"])
            with self.assertRaises(DataError):
                load_samples(path)

    def test_sorted_output(self):
        path = self._write_csv(
            ["s1,0.30,0.003\n", "s1,0.10,0.001\n", "s1,0.20,0.002\n"])
        sample = load_samples(path)[0]
        self.assertEqual(list(sample.pressures), [0.10, 0.20, 0.30])
        self.assertEqual(list(sample.quantities), [0.001, 0.002, 0.003])

    def test_config_validation(self):
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False,
                                         encoding="utf-8") as fh:
            json.dump({"cross_section_nm2": -1.0}, fh)
            bad = fh.name
        with self.assertRaises(DataError):
            load_config(bad)
        with tempfile.NamedTemporaryFile("w", suffix=".json", delete=False,
                                         encoding="utf-8") as fh:
            json.dump({"cross_section_nm2": SIGMA}, fh)
            good = fh.name
        cfg = load_config(good)
        self.assertEqual(cfg.min_points, 4)
        self.assertIsNone(cfg.interval)


class ReportTest(unittest.TestCase):
    def test_report_success_and_failure(self):
        from bet_fit.data import Sample
        ps, qs = make_sample()
        good = Sample("good", tuple(ps), tuple(qs))
        bad_ps = [0.05, 0.10, 0.15, 0.20, 0.25]
        bad_qs = [0.02 / (1 - p) * (1 - 0.5 * p) for p in bad_ps]
        bad = Sample("bad", tuple(bad_ps), tuple(bad_qs))
        text, ok = render_report([good, bad],
                                 Config(cross_section_nm2=SIGMA))
        self.assertFalse(ok)
        self.assertIn("surface area", text)
        self.assertIn("FAILED", text)
        self.assertIn("Candidate intervals", text)
        self.assertIn("residual", text)


if __name__ == "__main__":
    unittest.main()
