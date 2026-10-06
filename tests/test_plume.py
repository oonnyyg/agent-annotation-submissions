import json
import math
import subprocess
import sys
import tempfile
import unittest
from datetime import datetime
from pathlib import Path

from plume_exposure.engine import compute_contributions, summarize
from plume_exposure.io import (
    Config,
    ConfigError,
    Dispersion,
    MetPeriod,
    Receptor,
    Source,
    load_config,
    load_met_periods,
)
from plume_exposure.model import concentration, wind_components

DISP = Dispersion(0.5, 0.8, 0.4, 0.8)


def make_config(sources, receptors, decay=0.0):
    return Config("X", decay, sources, receptors, DISP)


def met(start_h, end_h, speed, direction):
    return MetPeriod(
        datetime(2026, 1, 1, start_h), datetime(2026, 1, 1, end_h), speed, direction
    )


class WindRotationTests(unittest.TestCase):
    def test_north_wind_blows_plume_south(self):
        down, cross = wind_components(0.0, -100.0, 0.0)
        self.assertAlmostEqual(down, 100.0)
        self.assertAlmostEqual(cross, 0.0)

    def test_east_wind_blows_plume_west(self):
        down, cross = wind_components(-100.0, 0.0, 90.0)
        self.assertAlmostEqual(down, 100.0)
        self.assertAlmostEqual(cross, 0.0, places=9)

    def test_rotation_invariance(self):
        src = Source("S", 0.0, 0.0, 10.0, 1.0)
        cfg_a = make_config([src], [Receptor("R", 0.0, -500.0, 0.0)])
        cfg_b = make_config([src], [Receptor("R", -500.0, 0.0, 0.0)])
        ca = compute_contributions(cfg_a, [met(0, 1, 5.0, 0.0)])[0]
        cb = compute_contributions(cfg_b, [met(0, 1, 5.0, 90.0)])[0]
        self.assertAlmostEqual(ca.concentration, cb.concentration, places=15)

    def test_upwind_receptor_gets_zero(self):
        down, _ = wind_components(0.0, 100.0, 0.0)
        self.assertLess(down, 0.0)
        c = concentration(1.0, 5.0, 10.0, 0.0, down, 0.0, 0.5, 0.8, 0.4, 0.8)
        self.assertEqual(c, 0.0)


class ReflectionTests(unittest.TestCase):
    def test_ground_receptor_sees_double_vertical(self):
        common = dict(
            emission_rate=1.0,
            wind_speed=5.0,
            source_height=0.0,
            downwind=1000.0,
            crosswind=0.0,
            sigma_y_coeff=0.5,
            sigma_y_exp=0.8,
            sigma_z_coeff=0.4,
            sigma_z_exp=0.8,
        )
        ground = concentration(receptor_height=0.0, **common)
        sigma_z = 0.4 * 1000.0**0.8
        expected = 2.0 / (2 * math.pi * 5.0 * (0.5 * 1000.0**0.8) * sigma_z)
        self.assertAlmostEqual(ground, expected, places=20)

    def test_elevated_source_reflection_adds(self):
        common = dict(
            emission_rate=1.0,
            wind_speed=5.0,
            source_height=50.0,
            receptor_height=0.0,
            downwind=1000.0,
            crosswind=0.0,
            sigma_y_coeff=0.5,
            sigma_y_exp=0.8,
            sigma_z_coeff=0.4,
            sigma_z_exp=0.8,
        )
        with_reflection = concentration(**common)
        sz = 0.4 * 1000.0**0.8
        sy = 0.5 * 1000.0**0.8
        single = (
            1.0
            / (2 * math.pi * 5.0 * sy * sz)
            * math.exp(-(50.0**2) / (2 * sz**2))
        )
        self.assertAlmostEqual(with_reflection, 2.0 * single, places=20)


class SummaryTests(unittest.TestCase):
    def test_unequal_duration_weighting_and_gap(self):
        src = Source("S", 0.0, 0.0, 0.0, 1.0)
        rec = Receptor("R", 0.0, -1000.0, 0.0)
        cfg = make_config([src], [rec])
        periods = [met(0, 1, 5.0, 0.0), met(2, 5, 10.0, 0.0)]
        contribs = compute_contributions(cfg, periods)
        (summary,) = summarize(cfg, periods, contribs)
        c1 = contribs[0].concentration
        c2 = contribs[1].concentration
        self.assertAlmostEqual(c2, c1 / 2.0, places=15)
        expected_avg = (c1 * 3600.0 + c2 * 3 * 3600.0) / (4 * 3600.0)
        self.assertAlmostEqual(summary.average_concentration, expected_avg, places=20)
        self.assertAlmostEqual(summary.total_duration, 4 * 3600.0)
        self.assertAlmostEqual(
            summary.time_integral, c1 * 3600.0 + c2 * 3 * 3600.0, places=12
        )
        self.assertEqual(summary.peak_period_index, 0)

    def test_peak_tie_picks_earliest(self):
        src = Source("S", 0.0, 0.0, 0.0, 1.0)
        rec = Receptor("R", 0.0, -1000.0, 0.0)
        cfg = make_config([src], [rec])
        periods = [met(0, 1, 5.0, 0.0), met(1, 2, 5.0, 0.0)]
        contribs = compute_contributions(cfg, periods)
        (summary,) = summarize(cfg, periods, contribs)
        self.assertEqual(summary.peak_period_index, 0)

    def test_decay_reduces_concentration(self):
        src = Source("S", 0.0, 0.0, 0.0, 1.0)
        rec = Receptor("R", 0.0, -1000.0, 0.0)
        periods = [met(0, 1, 5.0, 0.0)]
        plain = compute_contributions(make_config([src], [rec]), periods)[0]
        decayed = compute_contributions(
            make_config([src], [rec], decay=0.001), periods
        )[0]
        self.assertAlmostEqual(
            decayed.concentration,
            plain.concentration * math.exp(-0.001 * 1000.0 / 5.0),
            places=20,
        )


class ValidationTests(unittest.TestCase):
    def _write(self, tmp, name, text):
        path = Path(tmp) / name
        path.write_text(text, encoding="utf-8")
        return path

    def test_overlap_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self._write(
                tmp,
                "met.csv",
                "start,end,wind_speed,wind_dir\n"
                "2026-01-01T00:00,2026-01-01T02:00,5,0\n"
                "2026-01-01T01:00,2026-01-01T03:00,5,0\n",
            )
            with self.assertRaises(ConfigError):
                load_met_periods(path)

    def test_gap_allowed(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self._write(
                tmp,
                "met.csv",
                "start,end,wind_speed,wind_dir\n"
                "2026-01-01T00:00,2026-01-01T01:00,5,0\n"
                "2026-01-01T03:00,2026-01-01T04:00,5,0\n",
            )
            self.assertEqual(len(load_met_periods(path)), 2)

    def test_calm_wind_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self._write(
                tmp,
                "met.csv",
                "start,end,wind_speed,wind_dir\n"
                "2026-01-01T00:00,2026-01-01T01:00,0,0\n",
            )
            with self.assertRaises(ConfigError):
                load_met_periods(path)

    def test_negative_emission_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = {
                "pollutant": {"name": "X", "decay_rate": 0.0},
                "sources": [
                    {"id": "S", "easting": 0, "northing": 0, "height": 0,
                     "emission_rate": -1}
                ],
                "receptors": [{"id": "R", "easting": 0, "northing": -1, "height": 0}],
                "dispersion": {"sigma_y_coeff": 0.5, "sigma_y_exp": 0.8,
                               "sigma_z_coeff": 0.4, "sigma_z_exp": 0.8},
            }
            path = self._write(tmp, "cfg.json", json.dumps(cfg))
            with self.assertRaises(ConfigError):
                load_config(path)


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            cfg = {
                "pollutant": {"name": "SO2", "decay_rate": 0.0},
                "sources": [
                    {"id": "A", "easting": 0, "northing": 0, "height": 20,
                     "emission_rate": 2.0},
                    {"id": "B", "easting": 100, "northing": 0, "height": 10,
                     "emission_rate": 1.0},
                ],
                "receptors": [
                    {"id": "R1", "easting": 0, "northing": -500, "height": 0},
                    {"id": "R2", "easting": 200, "northing": -500, "height": 1.5},
                ],
                "dispersion": {"sigma_y_coeff": 0.5, "sigma_y_exp": 0.8,
                               "sigma_z_coeff": 0.4, "sigma_z_exp": 0.8},
            }
            (tmp_path / "cfg.json").write_text(json.dumps(cfg), encoding="utf-8")
            (tmp_path / "met.csv").write_text(
                "start,end,wind_speed,wind_dir\n"
                "2026-01-01T00:00,2026-01-01T01:00,5,0\n"
                "2026-01-01T02:00,2026-01-01T04:00,8,45\n",
                encoding="utf-8",
            )
            out = tmp_path / "out"
            result = subprocess.run(
                [sys.executable, "-m", "plume_exposure",
                 str(tmp_path / "cfg.json"), str(tmp_path / "met.csv"),
                 "--out-dir", str(out)],
                capture_output=True,
                text=True,
                cwd=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            lines = (out / "contributions.csv").read_text(encoding="utf-8").splitlines()
            self.assertEqual(len(lines), 1 + 8)
            summary = json.loads((out / "summary.json").read_text(encoding="utf-8"))
            self.assertEqual(summary["pollutant"], "SO2")
            self.assertEqual(len(summary["receptors"]), 2)
            self.assertEqual(summary["receptors"][0]["total_duration_s"], 3 * 3600.0)


if __name__ == "__main__":
    unittest.main()
