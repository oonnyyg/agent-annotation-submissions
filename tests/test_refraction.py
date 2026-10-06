"""Analytic tests for the refraction first-arrival analyser."""

import json
import math
import subprocess
import sys
import unittest

from refraction.analysis import dominance_intervals, first_arrivals
from refraction.model import Layer, VelocityModel
from refraction.paths import compute_paths


def two_layer_model():
    # v1 = 500 m/s over h = 10 m, half-space v2 = 2000 m/s.
    return VelocityModel([Layer(500.0, 10.0), Layer(2000.0, None)])


class ModelValidationTests(unittest.TestCase):
    def test_rejects_nonpositive_thickness(self):
        with self.assertRaises(ValueError):
            VelocityModel([Layer(500.0, 0.0), Layer(2000.0, None)])

    def test_rejects_non_increasing_velocity(self):
        with self.assertRaises(ValueError):
            VelocityModel([Layer(2000.0, 10.0), Layer(500.0, None)])

    def test_rejects_halfspace_thickness(self):
        with self.assertRaises(ValueError):
            VelocityModel([Layer(500.0, 10.0), Layer(2000.0, 5.0)])

    def test_single_halfspace_ok(self):
        model = VelocityModel([Layer(1000.0, None)])
        paths = compute_paths(model)
        self.assertEqual(len(paths), 1)
        self.assertEqual(paths[0].kind, "direct")


class TravelTimeTests(unittest.TestCase):
    def test_direct_wave(self):
        paths = compute_paths(two_layer_model())
        self.assertAlmostEqual(paths[0].travel_time(100.0), 0.2)

    def test_zero_offset(self):
        arrivals = first_arrivals([0.0], compute_paths(two_layer_model()))
        self.assertEqual(arrivals[0].time, 0.0)
        self.assertEqual(arrivals[0].paths[0].kind, "direct")

    def test_critical_distance_analytic(self):
        # x_c = 2 h tan(ic), sin(ic) = v1/v2 = 0.25.
        sin_i = 0.25
        expected = 2 * 10.0 * sin_i / math.sqrt(1 - sin_i**2)
        paths = compute_paths(two_layer_model())
        self.assertAlmostEqual(paths[1].critical_distance, expected)

    def test_refracted_below_critical_unreachable(self):
        paths = compute_paths(two_layer_model())
        refr = paths[1]
        self.assertFalse(refr.reachable(refr.critical_distance - 1e-3))
        self.assertTrue(refr.reachable(refr.critical_distance + 1e-3))

    def test_refracted_time_analytic(self):
        # t(x) = x/v2 + 2 h cos(ic)/v1.
        paths = compute_paths(two_layer_model())
        sin_i = 0.25
        cos_i = math.sqrt(1 - sin_i**2)
        x = 100.0
        expected = x / 2000.0 + 2 * 10.0 * cos_i / 500.0
        self.assertAlmostEqual(paths[1].travel_time(x), expected)

    def test_segment_times_sum_to_total(self):
        model = VelocityModel(
            [Layer(400.0, 5.0), Layer(900.0, 15.0), Layer(3000.0, None)]
        )
        for path in compute_paths(model):
            for offset in (0.0, 30.0, 250.0):
                if not path.reachable(offset):
                    continue
                segs = path.segments_at(offset)
                self.assertAlmostEqual(
                    sum(s.time for s in segs), path.travel_time(offset)
                )
                self.assertAlmostEqual(
                    sum(s.horizontal for s in segs), offset, places=6
                )


class DominanceTests(unittest.TestCase):
    def test_crossover_analytic(self):
        # Direct t = x/500; refracted t = x/2000 + 2 h cos(ic)/500.
        # Crossover: x = 2 h cos(ic) / (1 - 500/2000).
        sin_i = 0.25
        cos_i = math.sqrt(1 - sin_i**2)
        expected = 2 * 10.0 * cos_i / (1.0 - 0.25)
        paths = compute_paths(two_layer_model())
        dom = dominance_intervals(paths, 0.0, 200.0)
        self.assertEqual(len(dom["crossovers"]), 1)
        self.assertAlmostEqual(dom["crossovers"][0], expected, places=6)
        self.assertEqual(dom["intervals"][0]["winners"], ["direct"])
        self.assertEqual(dom["intervals"][1]["winners"], [2])

    def test_never_first_interface(self):
        # Thin fast middle layer whose head wave never wins.
        model = VelocityModel(
            [Layer(500.0, 10.0), Layer(1500.0, 0.5), Layer(4000.0, None)]
        )
        paths = compute_paths(model)
        dom = dominance_intervals(paths, 0.0, 500.0)
        self.assertIn(2, dom["never_first"])
        self.assertNotIn(3, dom["never_first"])


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        import tempfile, os

        payload = {
            "layers": [
                {"velocity": 500, "thickness": 10},
                {"velocity": 2000},
            ],
            "offsets": [0, 20, 100],
            "range": {"min": 0, "max": 200},
            "observed": [{"offset": 100, "time": 0.089}],
        }
        with tempfile.TemporaryDirectory() as tmp:
            inp = os.path.join(tmp, "in.json")
            csv_out = os.path.join(tmp, "paths.csv")
            json_out = os.path.join(tmp, "analysis.json")
            with open(inp, "w", encoding="utf-8") as fh:
                json.dump(payload, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "refraction", inp,
                 "--csv", csv_out, "--json", json_out],
                capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(json_out, encoding="utf-8") as fh:
                analysis = json.load(fh)
            self.assertIn("dominance", analysis)
            self.assertAlmostEqual(
                analysis["residuals"]["rms_s"],
                abs(0.089 - analysis["stations"][2]["first_arrival_time_s"]),
            )
            with open(csv_out, encoding="utf-8") as fh:
                self.assertIn("offset_m", fh.readline())

    def test_cli_rejects_bad_model(self):
        import tempfile, os

        payload = {"layers": [{"velocity": -5, "thickness": 1}], "offsets": []}
        with tempfile.TemporaryDirectory() as tmp:
            inp = os.path.join(tmp, "in.json")
            with open(inp, "w", encoding="utf-8") as fh:
                json.dump(payload, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "refraction", inp],
                capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 2)


if __name__ == "__main__":
    unittest.main()
