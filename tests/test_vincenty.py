import unittest

from route_sampler.model import WGS84
from route_sampler.vincenty import VincentyNotConverged
from route_sampler.vincenty import direct, inverse
from route_sampler.vincenty import normalize_azimuth, normalize_lon

A, F = WGS84.a, WGS84.f
TOL, MAX_ITER = 1e-12, 200


class TestNormalize(unittest.TestCase):
    def test_longitude_half_open(self):
        self.assertEqual(normalize_lon(180.0), -180.0)
        self.assertEqual(normalize_lon(-180.0), -180.0)
        self.assertEqual(normalize_lon(190.0), -170.0)
        self.assertEqual(normalize_lon(-190.0), 170.0)
        self.assertEqual(normalize_lon(540.0), -180.0)
        self.assertEqual(normalize_lon(0.0), 0.0)

    def test_azimuth(self):
        self.assertEqual(normalize_azimuth(-90.0), 270.0)
        self.assertEqual(normalize_azimuth(360.0), 0.0)
        self.assertEqual(normalize_azimuth(725.0), 5.0)


class TestInverse(unittest.TestCase):
    def test_known_equatorial_distance(self):
        result = inverse(0.0, 0.0, 0.0, 1.0, A, F, TOL, MAX_ITER)
        self.assertAlmostEqual(result.distance, 111319.4908, places=3)
        self.assertAlmostEqual(result.azimuth1, 90.0, places=6)

    def test_antimeridian_crossing(self):
        result = inverse(10.0, 179.5, 10.0, -179.5, A, F, TOL, MAX_ITER)
        self.assertAlmostEqual(result.distance, 109639.3, delta=0.5)
        self.assertAlmostEqual(result.azimuth1, 90.0, delta=0.2)

    def test_coincident_points_zero(self):
        result = inverse(30.0, 120.0, 30.0, 120.0, A, F, TOL, MAX_ITER)
        self.assertEqual(result.distance, 0.0)

    def test_near_antipodal_does_not_converge(self):
        with self.assertRaises(VincentyNotConverged):
            inverse(0.0, 0.0, 0.5, 179.7, A, F, TOL, 50)


class TestDirectInverseRoundTrip(unittest.TestCase):
    def test_round_trip(self):
        cases = [
            (31.2, 121.5, 35.7, 139.7),
            (-33.9, 151.2, 40.7, -74.0),
            (10.0, 179.9, -5.0, -179.8),
            (51.5, -0.1, 48.9, 2.3),
        ]
        for lat1, lon1, lat2, lon2 in cases:
            with self.subTest(lat1=lat1, lon1=lon1, lat2=lat2, lon2=lon2):
                inv = inverse(lat1, lon1, lat2, lon2, A, F, TOL, MAX_ITER)
                out = direct(lat1, lon1, inv.azimuth1, inv.distance, A, F, TOL, MAX_ITER)
                self.assertAlmostEqual(out.lat2, lat2, places=8)
                self.assertAlmostEqual(out.lon2, lon2, places=8)
                back = inverse(lat1, lon1, out.lat2, out.lon2, A, F, TOL, MAX_ITER)
                self.assertAlmostEqual(back.distance, inv.distance, places=6)

    def test_direct_zero_distance(self):
        out = direct(45.0, 90.0, 123.0, 0.0, A, F, TOL, MAX_ITER)
        self.assertAlmostEqual(out.lat2, 45.0, places=10)
        self.assertAlmostEqual(out.lon2, 90.0, places=10)


if __name__ == "__main__":
    unittest.main()
