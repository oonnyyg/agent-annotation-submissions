"""合成等温线与退化数据测试。"""

from __future__ import annotations

import json
import math
import tempfile
import unittest
from pathlib import Path

from betfit.cli import main
from betfit.fit import analyze_sample, enumerate_intervals, select_best
from betfit.io import BetInputError, Config, IntervalSpec, Sample, load_config, load_samples

SIGMA = 0.162  # N2 @ 77 K, nm^2


def bet_isotherm(ps, q_m, c):
    return [q_m * c * p / ((1 - p) * (1 - p + c * p)) for p in ps]


def make_sample(ps, qs, name="s"):
    return Sample(name=name, pressures=list(ps), loadings=list(qs))


def make_config(**kw):
    return Config(cross_section_nm2=kw.pop("sigma", SIGMA), **kw)


class SyntheticTest(unittest.TestCase):
    def setUp(self):
        self.ps = [0.01, 0.03, 0.05, 0.07, 0.10, 0.13, 0.16, 0.20, 0.25, 0.30]
        self.q_m, self.c = 2.5e-3, 120.0
        self.qs = bet_isotherm(self.ps, self.q_m, self.c)
        self.sample = make_sample(self.ps, self.qs)

    def test_recovers_parameters(self):
        res = analyze_sample(self.sample, make_config())
        self.assertTrue(res.ok)
        iv = res.selected
        self.assertAlmostEqual(iv.monolayer_capacity, self.q_m, places=8)
        self.assertAlmostEqual(iv.bet_constant, self.c, delta=1.0)
        expected_area = self.q_m * 6.02214076e23 * SIGMA * 1e-18
        self.assertAlmostEqual(iv.surface_area, expected_area, delta=1e-6)
        self.assertGreater(iv.r_squared, 0.9999)
        self.assertEqual(len(res.residuals), iv.n_points)

    def test_enumeration_count(self):
        ivs = enumerate_intervals(self.sample, 4, SIGMA)
        n = len(self.ps)
        self.assertEqual(len(ivs), sum(n - 4 + 1 - s for s in range(n - 4 + 1)))

    def test_tie_break_prefers_more_points(self):
        # 全部通过时，R^2 并列应选点数更多的区间
        cands = enumerate_intervals(self.sample, 4, SIGMA)
        best = select_best(cands)
        self.assertIsNotNone(best)
        top_r2 = max(c.r_squared for c in cands if c.accepted and c.r_squared is not None)
        tied = [c for c in cands if c.accepted and c.r_squared == top_r2]
        self.assertEqual(best.n_points, max(c.n_points for c in tied))


class DegenerateTest(unittest.TestCase):
    def test_constant_loading_no_auto_select(self):
        # y 恒定 -> ss_tot = 0 -> R^2 无法定义，不参与自动选择
        ps = [0.05, 0.10, 0.15, 0.20]
        qs = [p / (1 - p) for p in ps]  # 使 p/(q*(1-p)) = 1 恒定
        res = analyze_sample(make_sample(ps, qs), make_config())
        self.assertFalse(res.ok)
        self.assertTrue(all(c.r_squared is None for c in res.candidates))

    def test_rouquerol_violation_rejected(self):
        # q*(1-p) 上升 -> 所有区间被拒
        ps = [0.05, 0.10, 0.15, 0.20]
        qs = [0.002, 0.004, 0.009, 0.020]
        sample = make_sample(ps, qs)
        self.assertTrue(any(
            b > a for a, b in zip(
                [q * (1 - p) for p, q in zip(ps, qs)],
                [q * (1 - p) for p, q in zip(ps, qs)][1:],
            )
        ))
        res = analyze_sample(sample, make_config())
        self.assertFalse(res.ok)
        self.assertTrue(all(not c.accepted for c in res.candidates))

    def test_manual_interval_not_exempt(self):
        # 手动区间同样要过筛选
        ps = [0.05, 0.10, 0.15, 0.20]
        qs = [0.002, 0.004, 0.009, 0.020]
        cfg = make_config(intervals={"s": IntervalSpec(0.05, 0.20)})
        res = analyze_sample(make_sample(ps, qs), cfg)
        self.assertFalse(res.ok)
        self.assertEqual(len(res.candidates), 1)

    def test_manual_interval_too_few_points(self):
        ps = [0.05, 0.10, 0.15, 0.20, 0.30]
        qs = bet_isotherm(ps, 2.5e-3, 120.0)
        cfg = make_config(intervals={"s": IntervalSpec(0.14, 0.16)})
        res = analyze_sample(make_sample(ps, qs), cfg)
        self.assertFalse(res.ok)
        self.assertIn("点数", res.candidates[0].reasons[0])


class IoTest(unittest.TestCase):
    def _write(self, text, suffix):
        fh = tempfile.NamedTemporaryFile("w", suffix=suffix, delete=False, encoding="utf-8")
        fh.write(text)
        fh.close()
        return fh.name

    def test_csv_sorted_and_loaded(self):
        path = self._write(
            "p,a,b\n0.10,1.0,2.0\n0.05,0.5,1.0\n0.20,2.0,4.0\n", ".csv"
        )
        samples = load_samples(path)
        self.assertEqual([s.name for s in samples], ["a", "b"])
        self.assertEqual(samples[0].pressures, [0.05, 0.10, 0.20])
        self.assertEqual(samples[0].loadings, [0.5, 1.0, 2.0])

    def test_duplicate_pressure_located(self):
        path = self._write("p,a\n0.10,1.0\n0.10,2.0\n", ".csv")
        with self.assertRaisesRegex(BetInputError, "重复压力 0.1"):
            load_samples(path)

    def test_pressure_out_of_range(self):
        path = self._write("p,a\n1.5,1.0\n", ".csv")
        with self.assertRaisesRegex(BetInputError, r"\(0, 1\)"):
            load_samples(path)

    def test_non_finite_rejected(self):
        path = self._write("p,a\n0.1,nan\n", ".csv")
        with self.assertRaisesRegex(BetInputError, "非有限"):
            load_samples(path)

    def test_config_validation(self):
        bad = self._write(json.dumps({"cross_section_nm2": -1}), ".json")
        with self.assertRaises(BetInputError):
            load_config(bad)
        good = self._write(
            json.dumps({
                "cross_section_nm2": 0.162,
                "min_points": 5,
                "intervals": {"s": {"lower": 0.05, "upper": 0.2}},
            }),
            ".json",
        )
        cfg = load_config(good)
        self.assertEqual(cfg.min_points, 5)
        self.assertEqual(cfg.intervals["s"].upper, 0.2)


class CliTest(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as td:
            ps = [0.01, 0.03, 0.05, 0.07, 0.10, 0.13, 0.16, 0.20]
            qs1 = bet_isotherm(ps, 2.5e-3, 120.0)
            qs2 = bet_isotherm(ps, 4.0e-3, 80.0)
            csv_path = Path(td) / "data.csv"
            lines = ["p,s1,s2"] + [
                f"{p},{a},{b}" for p, a, b in zip(ps, qs1, qs2)
            ]
            csv_path.write_text("\n".join(lines), encoding="utf-8")
            cfg_path = Path(td) / "cfg.json"
            cfg_path.write_text(json.dumps({"cross_section_nm2": SIGMA}), encoding="utf-8")
            out_path = Path(td) / "report.txt"
            code = main([str(csv_path), str(cfg_path), "-o", str(out_path)])
            self.assertEqual(code, 0)
            report = out_path.read_text(encoding="utf-8")
            self.assertIn("样品 s1", report)
            self.assertIn("比表面积", report)
            self.assertIn("不构成完整的吸附适用性认证", report)

    def test_exit_code_on_failure(self):
        with tempfile.TemporaryDirectory() as td:
            csv_path = Path(td) / "data.csv"
            csv_path.write_text(
                "p,s\n0.05,0.002\n0.10,0.004\n0.15,0.009\n0.20,0.020\n",
                encoding="utf-8",
            )
            cfg_path = Path(td) / "cfg.json"
            cfg_path.write_text(json.dumps({"cross_section_nm2": SIGMA}), encoding="utf-8")
            code = main([str(csv_path), str(cfg_path)])
            self.assertEqual(code, 1)


if __name__ == "__main__":
    unittest.main()
