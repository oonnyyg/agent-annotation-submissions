import json
import math
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from point_pattern import (
    PatternError,
    PointPattern,
    RadiusGrid,
    compute_k,
    compute_l,
    simulate_envelope,
)


class TestValidation(unittest.TestCase):
    def test_rejects_nonpositive_dimensions(self):
        with self.assertRaises(PatternError):
            PointPattern(0.0, 1.0, ((0.1, 0.1), (0.2, 0.2)))
        with self.assertRaises(PatternError):
            PointPattern(1.0, -2.0, ((0.1, 0.1), (0.2, 0.2)))

    def test_rejects_out_of_bounds_and_boundary_edge(self):
        with self.assertRaises(PatternError):
            PointPattern(1.0, 1.0, ((1.0, 0.5), (0.2, 0.2)))
        with self.assertRaises(PatternError):
            PointPattern(1.0, 1.0, ((-0.1, 0.5), (0.2, 0.2)))

    def test_rejects_duplicate_points(self):
        with self.assertRaises(PatternError):
            PointPattern(1.0, 1.0, ((0.5, 0.5), (0.5, 0.5)))

    def test_rejects_single_point(self):
        with self.assertRaises(PatternError):
            PointPattern(1.0, 1.0, ((0.5, 0.5),))

    def test_grid_must_be_strictly_increasing_from_zero(self):
        with self.assertRaises(PatternError):
            RadiusGrid((0.0, 0.2, 0.2, 0.4), 0.5)
        with self.assertRaises(PatternError):
            RadiusGrid((0.1, 0.3), 0.5)
        with self.assertRaises(PatternError):
            RadiusGrid((0.0, 0.6), 0.5)

    def test_uniform_grid_reaches_half_shorter_side(self):
        grid = RadiusGrid.uniform(4.0, 2.0, 5)
        self.assertAlmostEqual(grid.radii[-1], 1.0)
        self.assertEqual(grid.radii[0], 0.0)


class TestKComputation(unittest.TestCase):
    def test_translation_weight_for_known_pair(self):
        # Unit square, points (0.1, 0.5) and (0.9, 0.5): |dx|=0.8, dy=0,
        # overlap = (1-0.8)*(1-0) = 0.2, weight = 5 per ordered pair.
        points = ((0.1, 0.5), (0.9, 0.5))
        radii = (0.0, 0.79, 0.8, 1.0)
        result = compute_k(1.0, 1.0, points, radii)
        # Pair distance exactly 0.8 is included at r=0.8 (<= radius).
        self.assertEqual(result["count"], [0.0, 0.0, 2.0, 2.0])
        self.assertAlmostEqual(result["k_corrected"][2], 10.0 / 2.0)
        self.assertAlmostEqual(result["k_uncorrected"][2], 2.0 / 2.0)

    def test_counts_are_ordered_pairs(self):
        points = ((0.1, 0.1), (0.2, 0.1), (0.3, 0.1))
        result = compute_k(1.0, 1.0, points, (0.0, 0.5))
        self.assertEqual(result["count"][1], 6.0)

    def test_l_is_sqrt_k_over_pi(self):
        k = [0.0, math.pi, 4.0 * math.pi]
        self.assertEqual(compute_l(k), [0.0, 1.0, 2.0])

    def test_zero_radius_gives_zero(self):
        points = ((0.1, 0.1), (0.9, 0.9))
        result = compute_k(1.0, 1.0, points, (0.0, 0.5))
        self.assertEqual(result["k_corrected"][0], 0.0)
        self.assertEqual(result["count"][0], 0.0)


class TestSimulation(unittest.TestCase):
    def test_seed_reproducibility(self):
        points = ((0.2, 0.2), (0.8, 0.8), (0.2, 0.8))
        radii = RadiusGrid.uniform(1.0, 1.0, 10).radii
        l_obs = compute_l(compute_k(1.0, 1.0, points, radii)["k_corrected"])
        first = simulate_envelope(1.0, 1.0, 3, radii, 19, 7, l_obs)
        second = simulate_envelope(1.0, 1.0, 3, radii, 19, 7, l_obs)
        self.assertEqual(first, second)
        third = simulate_envelope(1.0, 1.0, 3, radii, 19, 8, l_obs)
        self.assertNotEqual(first["envelope_max"], third["envelope_max"])

    def test_p_value_plus_one_bounds(self):
        points = ((0.2, 0.2), (0.8, 0.8))
        radii = RadiusGrid.uniform(1.0, 1.0, 8).radii
        l_obs = compute_l(compute_k(1.0, 1.0, points, radii)["k_corrected"])
        result = simulate_envelope(1.0, 1.0, 2, radii, 9, 3, l_obs)
        self.assertGreaterEqual(result["p_value"], 1.0 / 10.0)
        self.assertLessEqual(result["p_value"], 1.0)
        self.assertEqual(len(result["envelope_min"]), len(radii))
        for lo, hi in zip(result["envelope_min"], result["envelope_max"]):
            self.assertLessEqual(lo, hi)

    def test_rejects_zero_simulations(self):
        with self.assertRaises(PatternError):
            simulate_envelope(1.0, 1.0, 2, (0.0, 0.5), 0, 1, [0.0, 0.5])


class TestCli(unittest.TestCase):
    def test_end_to_end_and_seed_reproducibility(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "points.csv"
            csv_path.write_text(
                "x,y\n0.1,0.1\n0.9,0.1\n0.1,0.9\n0.9,0.9\n0.5,0.5\n",
                encoding="utf-8",
            )
            config = {
                "width": 1.0,
                "height": 1.0,
                "n_radii": 11,
                "simulations": 29,
                "seed": 42,
            }
            config_path = tmp_path / "config.json"
            config_path.write_text(json.dumps(config), encoding="utf-8")

            def run(out_dir):
                return subprocess.run(
                    [
                        sys.executable,
                        "-m",
                        "point_pattern",
                        str(csv_path),
                        str(config_path),
                        "--out-dir",
                        str(out_dir),
                    ],
                    capture_output=True,
                    text=True,
                )

            out1 = tmp_path / "out1"
            out2 = tmp_path / "out2"
            self.assertEqual(run(out1).returncode, 0)
            self.assertEqual(run(out2).returncode, 0)
            self.assertEqual(
                (out1 / "k_l_curves.csv").read_text(encoding="utf-8"),
                (out2 / "k_l_curves.csv").read_text(encoding="utf-8"),
            )
            self.assertEqual(
                (out1 / "simulation_test.json").read_text(encoding="utf-8"),
                (out2 / "simulation_test.json").read_text(encoding="utf-8"),
            )
            report = json.loads(
                (out1 / "simulation_test.json").read_text(encoding="utf-8")
            )
            self.assertEqual(report["n_simulations"], 29)
            self.assertEqual(report["seed"], 42)
            self.assertEqual(len(report["radii"]), 11)
            header = (
                (out1 / "k_l_curves.csv").read_text(encoding="utf-8").splitlines()[0]
            )
            self.assertIn("k_uncorrected", header)
            self.assertIn("l_corrected", header)

    def test_cli_rejects_duplicate_points(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "points.csv"
            csv_path.write_text("x,y\n0.5,0.5\n0.5,0.5\n", encoding="utf-8")
            config_path = tmp_path / "config.json"
            config_path.write_text(
                json.dumps({"width": 1.0, "height": 1.0}), encoding="utf-8"
            )
            proc = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "point_pattern",
                    str(csv_path),
                    str(config_path),
                ],
                capture_output=True,
                text=True,
            )
            self.assertEqual(proc.returncode, 2)
            self.assertIn("duplicate", proc.stderr)


if __name__ == "__main__":
    unittest.main()
