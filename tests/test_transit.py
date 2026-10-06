import math
import unittest

from transit_exposure import (
    LimbDarkening,
    TransitGeometry,
    exposure_averaged_flux,
    external_contact_interval,
    instantaneous_flux,
    total_stellar_flux,
)
from transit_exposure.core import IntegrationError, adaptive_simpson


def circle_overlap_area(r1, r2, d):
    if d >= r1 + r2:
        return 0.0
    if d <= abs(r1 - r2):
        return math.pi * min(r1, r2) ** 2
    a1 = r1 * r1 * math.acos((d * d + r1 * r1 - r2 * r2) / (2 * d * r1))
    a2 = r2 * r2 * math.acos((d * d + r2 * r2 - r1 * r1) / (2 * d * r2))
    a3 = 0.5 * math.sqrt(
        (-d + r1 + r2) * (d + r1 - r2) * (d - r1 + r2) * (d + r1 + r2)
    )
    return a1 + a2 - a3


UNIFORM = LimbDarkening(0.0, 0.0)


class TestLimbDarkening(unittest.TestCase):
    def test_total_flux_analytic(self):
        ld = LimbDarkening(0.3, 0.2)
        self.assertAlmostEqual(total_stellar_flux(ld), math.pi * (1 - 0.1 - 0.2 / 6))

    def test_intensity_bounds(self):
        ld = LimbDarkening(0.4, 0.3)
        self.assertAlmostEqual(ld.intensity(0.0), 1.0)
        self.assertAlmostEqual(ld.intensity(1.0), 1.0 - 0.4 - 0.3)

    def test_invalid_coefficients(self):
        with self.assertRaises(ValueError):
            LimbDarkening(-0.1, 0.0)
        with self.assertRaises(ValueError):
            LimbDarkening(0.6, 0.5)
        with self.assertRaises(ValueError):
            LimbDarkening(float("nan"), 0.0)


class TestUniformLimit(unittest.TestCase):
    """With u1 = u2 = 0 the loss must equal the pure area-overlap fraction."""

    def check(self, rp, b, t):
        geom = TransitGeometry(rp, b, 0.0, 1.0)
        flux = instantaneous_flux(UNIFORM, geom, t)
        d = geom.separation(t)
        expected = 1.0 - circle_overlap_area(1.0, rp, d) / math.pi
        self.assertAlmostEqual(flux, expected, places=9)

    def test_central_transit(self):
        self.check(0.1, 0.0, 0.0)

    def test_partial_overlap(self):
        self.check(0.2, 0.5, 0.3)

    def test_grazing(self):
        self.check(0.1, 1.05, 0.0)

    def test_planet_fully_inside(self):
        self.check(0.3, 0.2, 0.0)

    def test_no_overlap(self):
        geom = TransitGeometry(0.1, 2.0, 0.0, 1.0)
        self.assertEqual(instantaneous_flux(UNIFORM, geom, 0.0), 1.0)


class TestGeometry(unittest.TestCase):
    def test_external_contact(self):
        geom = TransitGeometry(0.1, 0.5, 3.0, 2.0)
        t1, t2 = external_contact_interval(geom)
        half = math.sqrt(1.1**2 - 0.5**2) / 2.0
        self.assertAlmostEqual(t1, 3.0 - half)
        self.assertAlmostEqual(t2, 3.0 + half)

    def test_tangent_is_empty(self):
        geom = TransitGeometry(0.1, 1.1, 0.0, 1.0)
        self.assertIsNone(external_contact_interval(geom))

    def test_no_transit_is_empty(self):
        geom = TransitGeometry(0.1, 1.5, 0.0, 1.0)
        self.assertIsNone(external_contact_interval(geom))

    def test_invalid_parameters(self):
        for args in [(0.0, 0.0, 0.0, 1.0), (1.1, 0.0, 0.0, 1.0),
                     (0.1, -1.0, 0.0, 1.0), (0.1, 0.0, 0.0, 0.0),
                     (0.1, 0.0, float("inf"), 1.0)]:
            with self.assertRaises(ValueError):
                TransitGeometry(*args)


class TestExposure(unittest.TestCase):
    def setUp(self):
        self.ld = LimbDarkening(0.3, 0.2)
        self.geom = TransitGeometry(0.1, 0.0, 5.0, 1.0)

    def test_zero_duration_is_instantaneous(self):
        t = 4.7
        self.assertAlmostEqual(
            exposure_averaged_flux(self.ld, self.geom, t, 0.0),
            instantaneous_flux(self.ld, self.geom, t),
            places=7,
        )

    def test_short_exposure_matches_instantaneous(self):
        mean = exposure_averaged_flux(self.ld, self.geom, 5.0, 1e-3)
        inst = instantaneous_flux(self.ld, self.geom, 5.0)
        self.assertAlmostEqual(mean, inst, places=6)

    def test_long_exposure_total_blocked_flux(self):
        # Time-integrated loss over a very long window equals the integral of
        # the blocked flux over the whole transit (computed independently).
        duration = 200.0
        mean = exposure_averaged_flux(self.ld, self.geom, 5.0, duration)
        n = 20001
        a, b = 5.0 - 2.0, 5.0 + 2.0
        h = (b - a) / (n - 1)
        loss = sum(
            1.0 - instantaneous_flux(self.ld, self.geom, a + i * h)
            for i in range(n)
        ) * h  # rectangular rule is fine for a smooth curve at 1e-4
        self.assertAlmostEqual(mean, 1.0 - loss / duration, places=5)

    def test_ingress_straddling_exposure(self):
        # Exposure crossing first contact must lie between endpoint values.
        t1, _ = external_contact_interval(self.geom)
        center = t1
        mean = exposure_averaged_flux(self.ld, self.geom, center, 0.4)
        before = instantaneous_flux(self.ld, self.geom, center - 0.2)
        after = instantaneous_flux(self.ld, self.geom, center + 0.2)
        self.assertLess(after, mean)
        self.assertLess(mean, before)

    def test_exposure_outside_transit(self):
        self.assertEqual(exposure_averaged_flux(self.ld, self.geom, 0.0, 1.0), 1.0)

    def test_negative_duration_rejected(self):
        with self.assertRaises(ValueError):
            exposure_averaged_flux(self.ld, self.geom, 5.0, -1.0)


class TestQuadrature(unittest.TestCase):
    def test_nonconvergence_reported(self):
        with self.assertRaises(IntegrationError):
            adaptive_simpson(lambda x: 1.0 / math.sqrt(x + 1e-12), 0.0, 1.0, 1e-15,
                             max_depth=5)

    def test_invalid_tolerance(self):
        with self.assertRaises(ValueError):
            adaptive_simpson(lambda x: x, 0.0, 1.0, 0.0)

    def test_known_integral(self):
        self.assertAlmostEqual(
            adaptive_simpson(math.sin, 0.0, math.pi, 1e-12), 2.0, places=12
        )


if __name__ == "__main__":
    unittest.main()
