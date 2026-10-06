import json
import math
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))

from annual_extremes.bootstrap import bootstrap_return_levels, percentile_linear
from annual_extremes.core import (
    EULER_MASCHERONI,
    build_annual_series,
    empirical_exceedance,
    fit_gumbel,
    gumbel_quantile,
    return_level,
)


class TestSeries(unittest.TestCase):
    def test_same_year_takes_max_and_missing_years_listed(self):
        series = build_annual_series(
            [(2000, 1.0), (2000, 3.0), (2001, 2.0), (2003, 4.0)]
        )
        self.assertEqual(series.years, (2000, 2001, 2003))
        self.assertEqual(series.values, (3.0, 2.0, 4.0))
        self.assertEqual(series.missing_years, (2002,))

    def test_too_few_years_rejected(self):
        with self.assertRaises(ValueError):
            build_annual_series([(2000, 1.0), (2001, 2.0)])

    def test_non_finite_rejected(self):
        with self.assertRaises(ValueError):
            build_annual_series(
                [(2000, 1.0), (2001, 2.0), (2002, float("nan"))]
            )


class TestFit(unittest.TestCase):
    def test_fit_formulas(self):
        values = [10.0, 12.0, 11.0, 13.0, 9.0]
        fit = fit_gumbel(values)
        mean = sum(values) / len(values)
        var = sum((v - mean) ** 2 for v in values) / (len(values) - 1)
        scale = math.sqrt(var) * math.sqrt(6.0) / math.pi
        self.assertAlmostEqual(fit.scale, scale)
        self.assertAlmostEqual(fit.location, mean - EULER_MASCHERONI * scale)

    def test_return_level_uses_non_exceedance(self):
        fit = fit_gumbel([10.0, 12.0, 11.0, 13.0, 9.0])
        t = 50.0
        expected = gumbel_quantile(fit, 1.0 - 1.0 / t)
        self.assertAlmostEqual(return_level(fit, t), expected)
        with self.assertRaises(ValueError):
            return_level(fit, 1.0)


class TestEmpirical(unittest.TestCase):
    def test_ties_get_average_rank(self):
        years = (2000, 2001, 2002, 2003)
        values = (5.0, 7.0, 7.0, 3.0)
        rows = empirical_exceedance(years, values)
        by_year = {row[0]: row for row in rows}
        self.assertEqual(by_year[2001][2], 1.5)
        self.assertEqual(by_year[2002][2], 1.5)
        self.assertAlmostEqual(by_year[2001][3], 1.5 / 5.0)
        self.assertEqual(by_year[2000][2], 3.0)
        self.assertEqual(by_year[2003][2], 4.0)
        self.assertEqual(rows[0][1], 7.0)


class TestBootstrap(unittest.TestCase):
    def test_fixed_seed_reproducible_and_order_independent(self):
        values = (10.0, 12.0, 11.0, 13.0, 9.0, 14.0, 10.5)
        a = bootstrap_return_levels(values, [10.0, 50.0, 100.0], 200, 42)
        b = bootstrap_return_levels(values, [10.0, 50.0, 100.0], 200, 42)
        c = bootstrap_return_levels(values, [100.0, 10.0, 50.0], 200, 42)
        self.assertEqual(a, b)
        self.assertEqual(
            {x.return_period: x for x in a},
            {x.return_period: x for x in c},
        )

    def test_degenerate_resamples_counted_not_dropped(self):
        values = (5.0, 5.0, 5.0, 6.0)
        result = bootstrap_return_levels(values, [10.0], 500, 7)[0]
        self.assertGreater(result.degenerate_count, 0)
        self.assertLessEqual(result.lower, result.upper)
        self.assertTrue(math.isfinite(result.lower))

    def test_percentile_linear(self):
        self.assertEqual(percentile_linear([1.0, 2.0, 3.0], 0.5), 2.0)
        self.assertAlmostEqual(percentile_linear([1.0, 2.0, 3.0], 0.25), 1.5)


class TestCLI(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            csv_path = tmp / "obs.csv"
            csv_path.write_text(
                "year,value\n"
                "2000,10\n2000,12\n2001,11\n2003,13\n2004,9\n2005,14\n",
                encoding="utf-8",
            )
            cfg_path = tmp / "cfg.json"
            cfg_path.write_text(
                json.dumps(
                    {
                        "return_periods": [10, 50, 100],
                        "bootstrap": {"resamples": 100, "seed": 1},
                    }
                ),
                encoding="utf-8",
            )
            series_out = tmp / "series.csv"
            stats_out = tmp / "stats.json"
            root = Path(__file__).resolve().parent.parent
            proc = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "annual_extremes",
                    str(csv_path),
                    str(cfg_path),
                    str(series_out),
                    str(stats_out),
                ],
                cwd=root,
                capture_output=True,
                text=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            stats = json.loads(stats_out.read_text(encoding="utf-8"))
            self.assertEqual(stats["missing_years"], [2002])
            self.assertEqual(stats["n_years"], 5)
            self.assertEqual(len(stats["return_levels"]), 3)
            for entry in stats["return_levels"]:
                ci = entry["ci95"]
                self.assertLessEqual(ci["lower"], entry["level"] or ci["upper"])
                self.assertLessEqual(ci["lower"], ci["upper"])
            lines = series_out.read_text(encoding="utf-8").strip().splitlines()
            self.assertEqual(len(lines), 1 + 5)
            self.assertEqual(
                lines[0], "year,value,rank,exceedance_probability"
            )

    def test_all_equal_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            csv_path = tmp / "obs.csv"
            csv_path.write_text(
                "year,value\n2000,5\n2001,5\n2002,5\n", encoding="utf-8"
            )
            cfg_path = tmp / "cfg.json"
            cfg_path.write_text(
                json.dumps({"return_periods": [10]}), encoding="utf-8"
            )
            root = Path(__file__).resolve().parent.parent
            proc = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "annual_extremes",
                    str(csv_path),
                    str(cfg_path),
                    str(tmp / "s.csv"),
                    str(tmp / "j.json"),
                ],
                cwd=root,
                capture_output=True,
                text=True,
            )
            self.assertEqual(proc.returncode, 1)
            self.assertIn("all equal", proc.stderr)


if __name__ == "__main__":
    unittest.main()
