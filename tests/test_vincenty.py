import math
import unittest

from ellipsoidal_route_sampler.vincenty import (
    WGS84,
    VincentyError,
    direct,
    inverse,
    normalize_bearing,
    normalize_lon,
)


class TestInverseDirect(unittest.TestCase):
    def test_known_distance(self):
        # Beijing to Shanghai, sanity range around 1067-1071 km.
        result = inverse(WGS84, 39.9, 116.4, 31.2, 121.5)
        self.assertTrue(1_060_000 < result.distance < 1_080_000)
        self.assertTrue(0.0 <= result.initial_bearing < 360.0)

    def test_round_trip(self):
        inv = inverse(WGS84, 51.5, -0.12, 35.68, 139.69)
        out = direct(WGS84, 51.5, -0.12, inv.initial_bearing, inv.distance)
        self.assertAlmostEqual(out.lat, 35.68, places=9)
        self.assertAlmostEqual(out.lon, 139.69, places=9)

    def test_dateline_crossing(self):
        result = inverse(WGS84, 10.0, 179.5, 10.0, -179.5)
        self.assertTrue(100_000 < result.distance < 120_000)
        out = direct(WGS84, 10.0, 179.5, result.initial_bearing,
                     result.distance)
        self.assertAlmostEqual(out.lon, -179.5, places=9)

    def test_zero_length_bearing_undefined(self):
        result = inverse(WGS84, 30.0, 120.0, 30.0, 120.0)
        self.assertEqual(result.distance, 0.0)
        self.assertTrue(math.isnan(result.initial_bearing))

    def test_normalisation(self):
        self.assertEqual(normalize_lon(180.0), -180.0)
        self.assertEqual(normalize_lon(-180.0), -180.0)
        self.assertEqual(normalize_lon(190.0), -170.0)
        self.assertEqual(normalize_bearing(360.0), 0.0)
        self.assertEqual(normalize_bearing(-10.0), 350.0)

    def test_non_convergence_raises(self):
        with self.assertRaises(VincentyError):
            inverse(WGS84, 0.0, 0.0, 0.5, 179.5, max_iter=3)

    def test_rejects_pole_and_non_finite(self):
        with self.assertRaises(ValueError):
            inverse(WGS84, 90.0, 0.0, 0.0, 0.0)
        with self.assertRaises(ValueError):
            inverse(WGS84, math.nan, 0.0, 0.0, 0.0)
        with self.assertRaises(ValueError):
            inverse(WGS84, 0.0, 180.0, 0.0, 0.0)


if __name__ == "__main__":
    unittest.main()
