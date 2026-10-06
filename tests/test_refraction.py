import json
import math
import subprocess
import sys
import unittest

from refraction import (
    Layer,
    ModelError,
    VelocityModel,
    branch_intervals,
    direct_path,
    first_arrivals,
    refracted_path,
    residual_report,
)


def two_layer():
    return VelocityModel([Layer(500.0, 100.0), Layer(2000.0)])


class ModelValidationTests(unittest.TestCase):
    def test_rejects_zero_thickness(self):
        with self.assertRaises(ModelError):
            VelocityModel([Layer(500.0, 0.0), Layer(2000.0)])

    def test_rejects_negative_thickness(self):
        with self.assertRaises(ModelError):
            VelocityModel([Layer(500.0, -5.0), Layer(2000.0)])

    def test_rejects_non_positive_velocity(self):
        with self.assertRaises(ModelError):
            VelocityModel([Layer(0.0, 10.0), Layer(2000.0)])

    def test_rejects_non_increasing_velocity(self):
        with self.assertRaises(ModelError):
            VelocityModel([Layer(2000.0, 10.0), Layer(2000.0)])

    def test_rejects_empty_model(self):
        with self.assertRaises(ModelError):
            VelocityModel([])

    def test_single_half_space_ok(self):
        model = VelocityModel([Layer(1500.0)])
        self.assertEqual(model.interface_count, 0)


class TravelTimeTests(unittest.TestCase):
    def test_direct_wave(self):
        model = two_layer()
        path = direct_path(model)
        self.assertAlmostEqual(path.time(250.0), 0.5)

    def test_zero_offset(self):
        best, tied = first_arrivals(two_layer(), 0.0)
        self.assertEqual(best, 0.0)
        self.assertEqual([p.kind for p in tied], ["direct"])

    def test_critical_distance_analytic(self):
        # x_crit = 2 h tan(theta_c), sin(theta_c) = v1/v2 = 0.25
        model = two_layer()
        path = refracted_path(model, 1)
        theta = math.asin(0.25)
        expected = 2 * 100.0 * math.tan(theta)
        self.assertAlmostEqual(path.min_offset, expected)

    def test_intercept_analytic(self):
        # t0 = 2 h cos(theta_c) / v1
        model = two_layer()
        path = refracted_path(model, 1)
        theta = math.asin(0.25)
        expected = 2 * 100.0 * math.cos(theta) / 500.0
        self.assertAlmostEqual(path.intercept, expected)

    def test_unreachable_below_critical_distance(self):
        model = two_layer()
        path = refracted_path(model, 1)
        self.assertFalse(path.reachable(path.min_offset - 1.0))
        self.assertTrue(path.reachable(path.min_offset + 1.0))
        best, tied = first_arrivals(model, path.min_offset - 1.0)
        self.assertEqual([p.kind for p in tied], ["direct"])

    def test_segment_times_sum_to_total(self):
        model = VelocityModel(
            [Layer(400.0, 30.0), Layer(900.0, 45.0), Layer(3000.0)]
        )
        for interface in (1, 2):
            path = refracted_path(model, interface)
            x = path.min_offset + 123.4
            total = sum(s.time for s in path.segments(x))
            self.assertAlmostEqual(total, path.time(x), places=9)
            horizontal = sum(s.horizontal for s in path.segments(x))
            self.assertAlmostEqual(horizontal, x, places=9)

    def test_tied_paths_kept_at_crossover(self):
        model = two_layer()
        refracted = refracted_path(model, 1)
        direct = direct_path(model)
        x_cross = refracted.intercept / (direct.slope - refracted.slope)
        _, tied = first_arrivals(model, x_cross)
        self.assertEqual(
            sorted(p.kind for p in tied), ["direct", "refracted"]
        )


class BranchTests(unittest.TestCase):
    def test_crossover_distance_analytic(self):
        model = two_layer()
        intervals, never_first = branch_intervals(model, 0.0, 1000.0)
        self.assertEqual(len(intervals), 2)
        theta = math.asin(0.25)
        t0 = 2 * 100.0 * math.cos(theta) / 500.0
        expected = t0 / (1 / 500.0 - 1 / 2000.0)
        self.assertAlmostEqual(intervals[0].end, expected)
        self.assertEqual(intervals[0].paths[0].kind, "direct")
        self.assertEqual(intervals[1].paths[0].interface, 1)
        self.assertEqual(never_first, [])

    def test_hidden_layer_never_first(self):
        model = VelocityModel(
            [Layer(500.0, 100.0), Layer(1500.0, 5.0), Layer(4000.0)]
        )
        intervals, never_first = branch_intervals(model, 0.0, 3000.0)
        self.assertEqual([p.interface for p in never_first], [1])
        winners = [
            (iv.paths[0].kind, iv.paths[0].interface) for iv in intervals
        ]
        self.assertEqual(
            winners, [("direct", None), ("refracted", 2)]
        )

    def test_single_half_space_single_branch(self):
        model = VelocityModel([Layer(1500.0)])
        intervals, never_first = branch_intervals(model, 0.0, 500.0)
        self.assertEqual(len(intervals), 1)
        self.assertEqual(never_first, [])


class ResidualTests(unittest.TestCase):
    def test_rms_and_mean(self):
        model = two_layer()
        picks = [(100.0, 0.21), (200.0, 0.39)]
        report = residual_report(model, picks)
        expected = [(0.21 - 0.2), (0.39 - 0.4)]
        rms = math.sqrt(sum(e * e for e in expected) / 2)
        self.assertAlmostEqual(report.rms, rms)
        self.assertAlmostEqual(report.mean, sum(expected) / 2)
        self.assertAlmostEqual(report.max_abs, 0.01)


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        import tempfile, pathlib

        payload = {
            "model": {
                "layers": [
                    {"velocity": 500, "thickness": 100},
                    {"velocity": 2000},
                ]
            },
            "offsets": [0, 100, 500],
            "range": [0, 1000],
            "observed": [[100, 0.2], [500, 0.6373]],
        }
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = pathlib.Path(tmp)
            src = tmp_path / "in.json"
            src.write_text(json.dumps(payload), encoding="utf-8")
            csv_path = tmp_path / "p.csv"
            json_path = tmp_path / "a.json"
            result = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "refraction",
                    str(src),
                    "--csv",
                    str(csv_path),
                    "--json",
                    str(json_path),
                ],
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            analysis = json.loads(json_path.read_text(encoding="utf-8"))
            self.assertEqual(len(analysis["arrivals"]), 3)
            self.assertEqual(
                len(analysis["branches"]["switch_points_m"]), 1
            )
            self.assertIn("rms_s", analysis["residuals"])
            rows = csv_path.read_text(encoding="utf-8").splitlines()
            self.assertTrue(rows[0].startswith("offset_m,path"))

    def test_invalid_model_exit_code(self):
        import tempfile, pathlib

        payload = {
            "model": {
                "layers": [
                    {"velocity": 2000, "thickness": 10},
                    {"velocity": 500},
                ]
            },
            "offsets": [10],
        }
        with tempfile.TemporaryDirectory() as tmp:
            src = pathlib.Path(tmp) / "in.json"
            src.write_text(json.dumps(payload), encoding="utf-8")
            result = subprocess.run(
                [sys.executable, "-m", "refraction", str(src)],
                capture_output=True,
                text=True,
            )
            self.assertEqual(result.returncode, 2)
            self.assertIn("error", result.stderr.lower())


if __name__ == "__main__":
    unittest.main()
