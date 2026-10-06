import unittest

from catenary_clearance.analysis import run
from catenary_clearance.clearance import min_clearance
from catenary_clearance.solver import CatenarySolution


def base_config(**overrides):
    cfg = {
        "supports": {"left": {"x": 0, "y": 20}, "right": {"x": 100, "y": 20}},
        "linear_density": 1.5,
        "gravity": 9.81,
        "terrain": [[-10, 0], [110, 0]],
        "cases": [{"name": "L120", "length": 120}],
    }
    cfg.update(overrides)
    return cfg


class ClearanceTest(unittest.TestCase):
    def test_flat_terrain_clearance_at_lowest_point(self):
        sol = CatenarySolution(0.0, 20.0, 100.0, 20.0, 120.0, 1.0)
        terrain = [(-10.0, -50.0), (110.0, -50.0)]
        result = min_clearance(sol, terrain)
        self.assertAlmostEqual(result["min_clearance"], sol.y(50.0) + 50.0, places=9)
        self.assertAlmostEqual(result["x"], 50.0, places=6)
        self.assertFalse(result["collision"])

    def test_interior_stationary_point_on_sloped_segment(self):
        sol = CatenarySolution(0.0, 20.0, 100.0, 20.0, 120.0, 1.0)
        terrain = [(-10.0, -12.0), (110.0, -14.0)]
        result = min_clearance(sol, terrain)
        self.assertTrue(1.0 < result["x"] < 99.0)
        brute = min(
            sol.y(i / 100.0) - (-12.0 - 2.0 * (i / 100.0 + 10.0) / 120.0)
            for i in range(0, 10001)
        )
        self.assertAlmostEqual(result["min_clearance"], brute, places=3)

    def test_negative_clearance_kept_and_flagged(self):
        sol = CatenarySolution(0.0, 20.0, 100.0, 20.0, 120.0, 1.0)
        terrain = [(-10.0, 0.0), (110.0, 19.0)]
        result = min_clearance(sol, terrain)
        self.assertLess(result["min_clearance"], 0.0)
        self.assertTrue(result["collision"])

    def test_extrema_independent_of_sample_count(self):
        cfg = base_config()
        report1, _, _ = run(dict(cfg, samples=11))
        report2, _, _ = run(dict(cfg, samples=5001))
        c1 = report1["cases"][0]
        c2 = report2["cases"][0]
        self.assertEqual(c1["clearance"]["min_clearance"],
                         c2["clearance"]["min_clearance"])
        self.assertEqual(c1["max_tension"], c2["max_tension"])
        self.assertEqual(c1["passed"], c2["passed"])

    def test_one_bad_case_does_not_block_others(self):
        cfg = base_config(cases=[
            {"name": "too_short", "length": 50},
            {"name": "ok", "length": 120},
        ])
        report, _, _ = run(cfg)
        self.assertFalse(report["cases"][0]["solved"])
        self.assertIn("error", report["cases"][0])
        self.assertTrue(report["cases"][1]["solved"])

    def test_limits_checked_per_case(self):
        cfg = base_config(
            cases=[{"name": "tight", "length": 101}, {"name": "slack", "length": 160}],
            max_tension=1.0,
            min_clearance=100.0,
        )
        report, _, _ = run(cfg)
        for case in report["cases"]:
            self.assertFalse(case["passed"])
            self.assertFalse(case["checks"]["max_tension"]["pass"])
            self.assertFalse(case["checks"]["min_clearance"]["pass"])


if __name__ == "__main__":
    unittest.main()
