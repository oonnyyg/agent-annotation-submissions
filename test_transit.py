import csv
import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from transit_model import TransitModel, adaptive_simpson


def uniform_overlap_area(d, radius):
    """Intersection area of unit disk with disk of given radius (Mandel-Agol)."""
    if d >= 1.0 + radius:
        return 0.0
    if d <= abs(1.0 - radius):
        return math.pi * min(1.0, radius) ** 2
    arg1 = min(1.0, max(-1.0, (d * d + radius * radius - 1.0) / (2 * d * radius)))
    arg2 = min(1.0, max(-1.0, (d * d + 1.0 - radius * radius) / (2 * d)))
    kite = (-d + radius + 1.0) * (d + radius - 1.0) * (d - radius + 1.0) * (
        d + radius + 1.0
    )
    return (
        radius * radius * math.acos(arg1)
        + math.acos(arg2)
        - 0.5 * math.sqrt(max(kite, 0.0))
    )


def make_model(**kwargs):
    params = dict(
        planet_radius=0.1,
        impact_parameter=0.3,
        t_mid_hours=10.0,
        velocity=0.5,
        u1=0.3,
        u2=0.2,
    )
    params.update(kwargs)
    return TransitModel(**params)


class TestValidation(unittest.TestCase):
    def test_rejects_bad_inputs(self):
        for bad in (
            dict(planet_radius=0.0),
            dict(planet_radius=1.5),
            dict(planet_radius=float("nan")),
            dict(impact_parameter=-0.1),
            dict(velocity=0.0),
            dict(u1=-0.1),
            dict(u1=0.6, u2=0.6),
            dict(tolerance=0.0),
            dict(t_mid_hours=float("inf")),
        ):
            with self.assertRaises(ValueError, msg=bad):
                make_model(**bad)

    def test_negative_duration_rejected(self):
        model = make_model()
        with self.assertRaises(ValueError):
            model.exposure_mean(10.0, -1.0)


class TestInstantaneous(unittest.TestCase):
    def test_no_overlap_returns_one(self):
        model = make_model()
        t = model.t_mid_hours - 100.0
        self.assertAlmostEqual(model.instantaneous(t).value, 1.0, places=12)

    def test_uniform_limit_matches_area_formula(self):
        model = make_model(u1=0.0, u2=0.0)
        for d in (0.0, 0.2, 0.5, 0.9, 0.95, 1.0, 1.05, 1.09):
            expected = 1.0 - uniform_overlap_area(d, model.planet_radius) / math.pi
            x = math.sqrt(max(d * d - model.impact_parameter**2, 0.0))
            t = model.t_mid_hours + x / model.velocity
            got = model.instantaneous(t).value
            self.assertAlmostEqual(got, expected, places=9, msg=f"d={d}")

    def test_full_coverage_gives_zero(self):
        model = make_model(planet_radius=1.0, impact_parameter=0.0)
        self.assertAlmostEqual(
            model.instantaneous(model.t_mid_hours).value, 0.0, places=12
        )

    def test_limb_darkening_changes_depth(self):
        flat = make_model(u1=0.0, u2=0.0, impact_parameter=0.0)
        dark = make_model(u1=0.5, u2=0.3, impact_parameter=0.0)
        # At mid-transit the planet covers the bright centre, so limb
        # darkening must deepen the loss relative to the uniform case.
        self.assertLess(
            dark.instantaneous(dark.t_mid_hours).value,
            flat.instantaneous(flat.t_mid_hours).value,
        )

    def test_symmetry(self):
        model = make_model()
        for dt in (0.1, 0.5, 1.3):
            a = model.instantaneous(model.t_mid_hours - dt).value
            b = model.instantaneous(model.t_mid_hours + dt).value
            self.assertAlmostEqual(a, b, places=12)


class TestContacts(unittest.TestCase):
    def test_contact_interval_positive_duration(self):
        model = make_model()
        t1, t4 = model.contact_interval()
        self.assertLess(t1, model.t_mid_hours)
        self.assertGreater(t4, model.t_mid_hours)
        expected_half = math.sqrt((1.1) ** 2 - 0.3**2) / 0.5
        self.assertAlmostEqual(t4 - t1, 2 * expected_half)

    def test_grazing_tangent_interval_empty(self):
        model = make_model(impact_parameter=1.1)  # exactly 1 + R
        self.assertIsNone(model.contact_interval())
        self.assertEqual(model.contact_times(), [])

    def test_miss_interval_empty(self):
        model = make_model(impact_parameter=1.5)
        self.assertIsNone(model.contact_interval())
        self.assertAlmostEqual(
            model.instantaneous(model.t_mid_hours).value, 1.0, places=12
        )


class TestExposureMean(unittest.TestCase):
    def test_zero_duration_is_instantaneous(self):
        model = make_model()
        t = model.t_mid_hours + 0.2
        self.assertEqual(
            model.exposure_mean(t, 0.0).value, model.instantaneous(t).value
        )

    def test_long_exposure_tends_to_one(self):
        model = make_model()
        mean = model.exposure_mean(model.t_mid_hours, 1000.0)
        self.assertTrue(mean.converged)
        self.assertGreater(mean.value, 0.999)
        self.assertLess(mean.value, 1.0)

    def test_ingress_straddling_exposure(self):
        model = make_model()
        t1, _ = model.contact_interval()
        mean = model.exposure_mean(t1, 0.4).value
        inst_inside = model.instantaneous(t1 + 0.2).value
        self.assertLess(mean, 1.0)
        self.assertGreater(mean, inst_inside)

    def test_short_exposure_matches_instantaneous(self):
        model = make_model()
        t = model.t_mid_hours + 0.3
        mean = model.exposure_mean(t, 1e-4).value
        inst = model.instantaneous(t).value
        self.assertAlmostEqual(mean, inst, places=6)

    def test_mean_matches_quadrature_of_instantaneous(self):
        model = make_model()
        center, duration = model.t_mid_hours + 0.1, 0.7
        mean = model.exposure_mean(center, duration).value
        ref = adaptive_simpson(
            lambda t: model.instantaneous(t).value,
            center - duration / 2,
            center + duration / 2,
            1e-12,
        )
        self.assertAlmostEqual(mean, ref.value / duration, places=9)


class TestConvergenceReporting(unittest.TestCase):
    def test_nonconvergence_is_reported_not_clamped(self):
        model = make_model(tolerance=1e-16, max_depth=1)
        result = model.instantaneous(model.t_mid_hours)
        self.assertFalse(result.converged)
        self.assertFalse(model.diagnostics["converged"])


class TestCli(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            config = {
                "planet_radius_ratio": 0.1,
                "impact_parameter": 0.3,
                "t_mid_hours": 10.0,
                "velocity": 0.5,
                "limb_darkening": [0.3, 0.2],
                "tolerance": 1e-10,
            }
            cfg_path = os.path.join(tmp, "config.json")
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump(config, fh)
            exp_path = os.path.join(tmp, "exposures.csv")
            with open(exp_path, "w", encoding="utf-8", newline="") as fh:
                writer = csv.writer(fh)
                writer.writerow(["center_hours", "duration_hours"])
                writer.writerow([10.0, 0.0])
                writer.writerow([10.0, 0.5])
                writer.writerow([0.0, 1.0])
            out_csv = os.path.join(tmp, "out.csv")
            out_json = os.path.join(tmp, "diag.json")
            proc = subprocess.run(
                [sys.executable, "transit_cli.py", cfg_path, exp_path,
                 out_csv, out_json],
                capture_output=True,
                text=True,
                cwd=os.path.dirname(os.path.abspath(__file__)),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(out_csv, encoding="utf-8") as fh:
                rows = list(csv.DictReader(fh))
            self.assertEqual(len(rows), 3)
            self.assertLess(float(rows[0]["instantaneous"]), 1.0)
            self.assertAlmostEqual(
                float(rows[0]["instantaneous"]),
                float(rows[0]["exposure_mean"]),
                places=12,
            )
            self.assertAlmostEqual(float(rows[2]["exposure_mean"]), 1.0, places=12)
            with open(out_json, encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertTrue(diag["converged"])
            self.assertIsNotNone(diag["contact_interval_hours"])


if __name__ == "__main__":
    unittest.main()
