import math
import unittest

from catenary_clearance import (
    CatenaryError,
    clearance_against_terrain,
    solve_catenary,
)
from catenary_clearance.analysis import analyze_config

W = 1.2 * 9.81  # weight per unit length


class SolverTests(unittest.TestCase):
    def test_level_supports(self):
        cat = solve_catenary(0.0, 10.0, 80.0, 10.0, 100.0, W)
        self.assertAlmostEqual(cat.y(0.0), 10.0, places=9)
        self.assertAlmostEqual(cat.y(80.0), 10.0, places=9)
        self.assertAlmostEqual(cat.x_vertex, 40.0, places=7)
        res = cat.residuals()
        self.assertLess(res["arc_length"], 1e-8)
        self.assertAlmostEqual(cat.tension_at(40.0), cat.horizontal_tension, places=6)
        t_left, t_right = cat.support_tensions
        self.assertAlmostEqual(t_left, t_right, places=9)

    def test_unequal_supports(self):
        cat = solve_catenary(0.0, 5.0, 60.0, 25.0, 90.0, W)
        self.assertAlmostEqual(cat.y(0.0), 5.0, places=9)
        self.assertAlmostEqual(cat.y(60.0), 25.0, places=9)
        res = cat.residuals()
        self.assertLess(res["arc_length"], 1e-8)
        t_left, t_right = cat.support_tensions
        self.assertGreater(t_right, t_left)
        self.assertAlmostEqual(cat.max_tension, t_right, places=9)

    def test_nearly_taut(self):
        chord = math.hypot(80.0, 10.0)
        cat = solve_catenary(0.0, 0.0, 80.0, 10.0, chord * 1.000001, W)
        res = cat.residuals()
        self.assertLess(res["arc_length"], 1e-6)
        self.assertLess(res["endpoint_left"], 1e-6)

    def test_deep_sag(self):
        cat = solve_catenary(0.0, 0.0, 50.0, 0.0, 500.0, W)
        res = cat.residuals()
        self.assertLess(res["arc_length"], 1e-8)
        self.assertLess(cat.y(25.0), -100.0)

    def test_vertex_outside_span(self):
        cat = solve_catenary(0.0, 0.0, 40.0, 30.0, 51.0, W)
        self.assertFalse(cat.x1 <= cat.x_vertex <= cat.x2)
        x, y = cat.span_lowest_point()
        self.assertEqual(x, 0.0)
        self.assertEqual(y, 0.0)

    def test_invalid_inputs(self):
        with self.assertRaises(CatenaryError):
            solve_catenary(0.0, 0.0, 10.0, 0.0, 9.0, W)
        with self.assertRaises(CatenaryError):
            solve_catenary(0.0, 0.0, 10.0, 0.0, 10.0, W)
        with self.assertRaises(CatenaryError):
            solve_catenary(0.0, 0.0, 0.0, 0.0, 5.0, W)
        with self.assertRaises(CatenaryError):
            solve_catenary(0.0, 0.0, 10.0, 0.0, 20.0, -1.0)


class ClearanceTests(unittest.TestCase):
    def setUp(self):
        self.cat = solve_catenary(0.0, 10.0, 80.0, 10.0, 100.0, W)

    def test_flat_terrain(self):
        terrain = [(-10.0, -50.0), (90.0, -50.0)]
        res = clearance_against_terrain(self.cat, terrain)
        # Minimum at the vertex (x=40), exact, not sampled.
        self.assertAlmostEqual(res.x, 40.0, places=7)
        self.assertAlmostEqual(res.min_clearance, self.cat.y(40.0) + 50.0, places=9)
        self.assertFalse(res.collision)

    def test_interior_stationary_point(self):
        # Sloped terrain whose minimum lies strictly inside a segment.
        terrain = [(0.0, 8.0), (80.0, 4.0)]
        res = clearance_against_terrain(self.cat, terrain)
        self.assertTrue(0.0 < res.x < 80.0)
        # Cross-check with dense sampling.
        best = min(
            self.cat.y(i * 0.001) - (8.0 - 4.0 * i * 0.001 / 80.0)
            for i in range(0, 80001)
        )
        self.assertAlmostEqual(res.min_clearance, best, places=4)

    def test_collision_keeps_negative(self):
        terrain = [(0.0, 9.9), (80.0, 9.9)]
        res = clearance_against_terrain(self.cat, terrain)
        self.assertTrue(res.collision)
        self.assertLess(res.min_clearance, 0.0)

    def test_terrain_validation(self):
        with self.assertRaises(CatenaryError):
            clearance_against_terrain(self.cat, [(0.0, 0.0), (50.0, 0.0)])
        with self.assertRaises(CatenaryError):
            clearance_against_terrain(self.cat, [(10.0, 0.0), (5.0, 0.0), (90.0, 0.0)])


class AnalysisTests(unittest.TestCase):
    def _config(self, lengths, **limits):
        return {
            "span": {"x1": 0.0, "y1": 10.0, "x2": 80.0, "y2": 14.0},
            "linear_density": 1.2,
            "gravity": 9.81,
            "terrain": [[-5.0, 0.0], [40.0, 2.0], [85.0, 0.0]],
            "lengths": lengths,
            "limits": limits,
        }

    def test_multiple_lengths_independent(self):
        report = analyze_config(self._config(
            [90.0, 50.0, 120.0], max_tension=1e9, min_clearance=0.0
        ))
        scenarios = report["scenarios"]
        self.assertTrue(scenarios[0]["ok"])
        self.assertFalse(scenarios[1]["ok"])  # shorter than chord
        self.assertIn("error", scenarios[1])
        self.assertTrue(scenarios[2]["ok"])

    def test_limits_judged_per_scenario(self):
        report = analyze_config(self._config(
            [90.0, 200.0], max_tension=1.0, min_clearance=0.0
        ))
        s1, s2 = report["scenarios"]
        self.assertFalse(s1["checks"]["max_tension"]["pass"])
        self.assertFalse(s1["pass"])
        self.assertFalse(s2["checks"]["min_clearance"]["pass"])
        self.assertTrue(s2["clearance"]["collision"])

    def test_vertex_reported_outside_span(self):
        config = self._config([90.0])
        config["span"] = {"x1": 0.0, "y1": 0.0, "x2": 40.0, "y2": 30.0}
        config["lengths"] = [51.0]
        config["terrain"] = [[-5.0, -5.0], [45.0, -5.0]]
        report = analyze_config(config)
        s = report["scenarios"][0]
        self.assertFalse(s["vertex"]["inside_span"])
        self.assertEqual(s["span_lowest_point"]["x"], 0.0)


if __name__ == "__main__":
    unittest.main()
