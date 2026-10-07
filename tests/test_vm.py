import math
import unittest

from circular_mixture.vm import (
    KAPPA_MAX,
    a1,
    bessel_i0,
    circular_mean,
    kappa_from_r,
    log_density,
    norm_angle,
)


class TestNormAngle(unittest.TestCase):
    def test_full_turns_coincide(self):
        base = norm_angle(0.3)
        self.assertAlmostEqual(norm_angle(0.3 + 2 * math.pi), base)
        self.assertAlmostEqual(norm_angle(0.3 - 4 * math.pi), base)

    def test_range(self):
        for deg in (-720, -1, 0, 359.999, 360, 721):
            r = norm_angle(math.radians(deg))
            self.assertGreaterEqual(r, 0.0)
            self.assertLess(r, 2 * math.pi)


class TestBessel(unittest.TestCase):
    def test_i0_known_values(self):
        self.assertAlmostEqual(bessel_i0(0.0), 1.0)
        self.assertAlmostEqual(bessel_i0(1.0), 1.2660658777520084, places=12)
        self.assertAlmostEqual(bessel_i0(10.0), 2815.716628466254, places=4)

    def test_a1_bounds(self):
        self.assertEqual(a1(0.0), 0.0)
        prev = 0.0
        for k in (0.5, 1.0, 5.0, 20.0, 50.0):
            v = a1(k)
            self.assertGreater(v, prev)
            self.assertLess(v, 1.0)
            prev = v


class TestCircularMean(unittest.TestCase):
    def test_wraparound_cluster(self):
        # Samples at 358 and 2 degrees must average near 0, not 180.
        angles = [math.radians(358.0), math.radians(2.0)]
        mu, r = circular_mean(angles, [1.0, 1.0])
        self.assertAlmostEqual(r, math.cos(math.radians(2.0)), places=12)
        # mu near 0 (or equivalently 2*pi)
        self.assertTrue(mu < 0.01 or mu > 2 * math.pi - 0.01)

    def test_uniform_gives_zero_r(self):
        angles = [i * math.pi / 2 for i in range(4)]
        mu, r = circular_mean(angles, [1.0] * 4)
        self.assertAlmostEqual(r, 0.0, places=15)

    def test_zero_total_weight_rejected(self):
        with self.assertRaises(ValueError):
            circular_mean([0.0], [0.0])


class TestKappa(unittest.TestCase):
    def test_zero_r_is_uniform(self):
        self.assertEqual(kappa_from_r(0.0), 0.0)

    def test_capped(self):
        self.assertEqual(kappa_from_r(1.0), KAPPA_MAX)
        self.assertLessEqual(kappa_from_r(0.9999), KAPPA_MAX)


class TestLogDensity(unittest.TestCase):
    def test_uniform(self):
        self.assertAlmostEqual(log_density(1.0, 2.0, 0.0), -math.log(2 * math.pi))

    def test_normalized(self):
        # Numerically integrate density over the circle.
        kappa, mu = 3.0, 1.0
        n = 20000
        total = 0.0
        for i in range(n):
            t = 2 * math.pi * (i + 0.5) / n
            total += math.exp(log_density(t, mu, kappa))
        total *= 2 * math.pi / n
        self.assertAlmostEqual(total, 1.0, places=8)


if __name__ == "__main__":
    unittest.main()
