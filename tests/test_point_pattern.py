import math
import unittest

from point_pattern.core import (
    Window,
    k_estimate,
    l_from_k,
    ordered_pair_distances,
    translation_weight,
    validate_points,
    validate_radii,
)
from point_pattern.simulate import (
    deviation_statistic,
    simulate_l_curves,
    upper_tail_pvalue,
)


class WindowTests(unittest.TestCase):
    def test_positive_sides_required(self):
        with self.assertRaises(ValueError):
            Window(0.0, 1.0)
        with self.assertRaises(ValueError):
            Window(1.0, -2.0)

    def test_max_radius_is_half_shorter_side(self):
        self.assertEqual(Window(10.0, 6.0).max_radius, 3.0)


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.window = Window(10.0, 10.0)

    def test_out_of_bounds_rejected(self):
        with self.assertRaises(ValueError):
            validate_points([(10.0, 5.0), (1.0, 1.0)], self.window)
        with self.assertRaises(ValueError):
            validate_points([(-0.1, 5.0), (1.0, 1.0)], self.window)

    def test_boundary_origin_side_accepted(self):
        pts = validate_points([(0.0, 0.0), (9.999, 5.0)], self.window)
        self.assertEqual(len(pts), 2)

    def test_duplicates_rejected(self):
        with self.assertRaises(ValueError):
            validate_points([(1.0, 1.0), (1.0, 1.0)], self.window)

    def test_at_least_two_points(self):
        with self.assertRaises(ValueError):
            validate_points([(1.0, 1.0)], self.window)

    def test_radii_must_start_at_zero_and_increase(self):
        with self.assertRaises(ValueError):
            validate_radii([0.1, 0.2], self.window)
        with self.assertRaises(ValueError):
            validate_radii([0.0, 0.0, 1.0], self.window)
        with self.assertRaises(ValueError):
            validate_radii([0.0, 6.0], self.window)
        ok = validate_radii([0.0, 2.5, 5.0], self.window)
        self.assertEqual(ok, [0.0, 2.5, 5.0])


class WeightTests(unittest.TestCase):
    def test_translation_weight(self):
        w = Window(10.0, 10.0)
        self.assertAlmostEqual(translation_weight(w, 0.0, 0.0), 1.0)
        self.assertAlmostEqual(translation_weight(w, 2.0, 0.0), 100.0 / 80.0)
        self.assertAlmostEqual(translation_weight(w, 5.0, 5.0), 100.0 / 25.0)


class KEstimateTests(unittest.TestCase):
    def setUp(self):
        self.window = Window(10.0, 10.0)
        self.points = [(1.0, 1.0), (4.0, 1.0), (4.0, 5.0)]
        self.pairs = ordered_pair_distances(self.points)

    def test_ordered_pairs_counted(self):
        self.assertEqual(len(self.pairs), 6)

    def test_radius_endpoint_inclusive(self):
        counts, _ = k_estimate(
            self.window, self.pairs, [0.0, 3.0, 4.0, 5.0], correct=False
        )
        # distances: 3, 3, 4, 4, 5, 5
        self.assertEqual(counts, [0, 2, 4, 6])

    def test_uncorrected_k(self):
        _, k = k_estimate(self.window, self.pairs, [0.0, 3.0], correct=False)
        self.assertAlmostEqual(k[0], 0.0)
        self.assertAlmostEqual(k[1], 100.0 * 2 / 6.0)

    def test_corrected_k_applies_weights(self):
        _, k = k_estimate(self.window, self.pairs, [0.0, 3.0], correct=True)
        w = translation_weight(self.window, 3.0, 0.0)
        self.assertAlmostEqual(k[1], 100.0 * 2 * w / 6.0)

    def test_l_from_k(self):
        self.assertAlmostEqual(l_from_k([math.pi * 4.0])[0], 2.0)


class SimulationTests(unittest.TestCase):
    def test_seed_reproducibility(self):
        w = Window(10.0, 8.0)
        radii = [0.0, 1.0, 2.0, 4.0]
        a = simulate_l_curves(w, radii, 12, 5, seed=42)
        b = simulate_l_curves(w, radii, 12, 5, seed=42)
        self.assertEqual(a, b)
        c = simulate_l_curves(w, radii, 12, 5, seed=43)
        self.assertNotEqual(a, c)

    def test_statistic_and_pvalue(self):
        self.assertAlmostEqual(
            deviation_statistic([0.5, 2.5], [0.0, 2.0]), 0.5
        )
        # ties count towards the tail, plus-one correction
        self.assertAlmostEqual(upper_tail_pvalue(1.0, [1.0, 0.5, 2.0]), 3 / 4)


if __name__ == "__main__":
    unittest.main()
