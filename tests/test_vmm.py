import math
import unittest

from circular_mixture_fit.vmm import (
    a1_inverse,
    bessel_i0,
    circular_mean,
    normalize_angle,
)


class NormalizeTests(unittest.TestCase):
    def test_whole_turns_coincide(self):
        a = normalize_angle(math.radians(5.0))
        b = normalize_angle(math.radians(365.0))
        c = normalize_angle(math.radians(-355.0))
        self.assertAlmostEqual(a, b, places=12)
        self.assertAlmostEqual(a, c, places=12)

    def test_range(self):
        for deg in (-720.0, -0.1, 0.0, 179.9, 359.999, 360.0, 1080.5):
            v = normalize_angle(math.radians(deg))
            self.assertGreaterEqual(v, 0.0)
            self.assertLess(v, 2.0 * math.pi)


class CircularMeanTests(unittest.TestCase):
    def test_wraparound_cluster_not_averaged_to_opposite(self):
        angles = [math.radians(359.0), math.radians(1.0), math.radians(358.5)]
        mu, r = circular_mean(angles, [1.0, 1.0, 1.0])
        mu_deg = math.degrees(mu)
        self.assertTrue(mu_deg > 350.0 or mu_deg < 10.0)
        self.assertGreater(r, 0.99)

    def test_uniform_directions_give_zero_resultant(self):
        angles = [math.radians(d) for d in (0.0, 90.0, 180.0, 270.0)]
        _, r = circular_mean(angles, [1.0] * 4)
        self.assertAlmostEqual(r, 0.0, places=12)

    def test_weights(self):
        angles = [0.0, math.pi / 2.0]
        mu, _ = circular_mean(angles, [3.0, 1.0])
        self.assertLess(mu, math.pi / 4.0)


class BesselTests(unittest.TestCase):
    def test_i0_known_values(self):
        self.assertAlmostEqual(bessel_i0(0.0), 1.0, places=15)
        self.assertAlmostEqual(bessel_i0(1.0), 1.2660658777520084, places=12)
        self.assertAlmostEqual(bessel_i0(10.0), 2815.7166284662544, places=6)

    def test_a1_inverse_bounds(self):
        self.assertEqual(a1_inverse(0.0), 0.0)
        self.assertLessEqual(a1_inverse(0.9999999), 50.0)
        k = a1_inverse(0.5)
        self.assertGreater(k, 0.0)
        self.assertLess(k, 50.0)


if __name__ == "__main__":
    unittest.main()
