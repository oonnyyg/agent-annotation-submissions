import math
import unittest

from plume_exposure.model import (
    DispersionCoefficients,
    concentration,
    wind_relative_coordinates,
)

COEFFS = DispersionCoefficients(
    lateral_coeff=0.5, lateral_exponent=0.9, vertical_coeff=0.3, vertical_exponent=0.8
)


class WindRotationTests(unittest.TestCase):
    def test_north_wind_blows_south(self):
        # Wind FROM north (0 deg): receptor south of source is downwind.
        downwind, crosswind = wind_relative_coordinates(0.0, -100.0, 0.0)
        self.assertAlmostEqual(downwind, 100.0)
        self.assertAlmostEqual(crosswind, 0.0)

    def test_east_wind_blows_west(self):
        downwind, crosswind = wind_relative_coordinates(-50.0, 0.0, 90.0)
        self.assertAlmostEqual(downwind, 50.0)
        self.assertAlmostEqual(crosswind, 0.0, places=9)

    def test_rotation_invariance(self):
        # Rotating geometry and wind together keeps coordinates identical.
        dx, dy = 30.0, -40.0
        base = wind_relative_coordinates(dx, dy, 10.0)
        # Compass angles are clockwise from north; rotate the offset the
        # same way (clockwise by 60 deg) and add 60 deg to the wind.
        cos, sin = math.cos(math.radians(60.0)), math.sin(math.radians(60.0))
        rotated = wind_relative_coordinates(
            dx * cos + dy * sin, -dx * sin + dy * cos, 70.0
        )
        self.assertAlmostEqual(base[0], rotated[0])
        self.assertAlmostEqual(base[1], rotated[1])

    def test_upwind_receptor_zero(self):
        value = concentration(
            emission_rate=1.0,
            wind_speed=2.0,
            downwind_m=-10.0,
            crosswind_m=0.0,
            source_height_m=10.0,
            receptor_height_m=0.0,
            coeffs=COEFFS,
        )
        self.assertEqual(value, 0.0)


class ReflectionTests(unittest.TestCase):
    def test_ground_receptor_doubles_vertical_term(self):
        # At z_r = 0, direct and reflected images coincide: factor of 2
        # relative to a single (direct-only) plume.
        common = dict(
            emission_rate=1.0,
            wind_speed=2.0,
            downwind_m=500.0,
            crosswind_m=0.0,
            source_height_m=20.0,
            coeffs=COEFFS,
        )
        with_reflection = concentration(receptor_height_m=0.0, **common)
        sigma_z = COEFFS.sigma_z(500.0)
        direct_only = (
            1.0
            / (2.0 * math.pi * 2.0 * COEFFS.sigma_y(500.0) * sigma_z)
            * math.exp(-0.5 * (20.0 / sigma_z) ** 2)
        )
        self.assertAlmostEqual(with_reflection, 2.0 * direct_only)

    def test_decay_reduces_concentration(self):
        common = dict(
            emission_rate=1.0,
            wind_speed=2.0,
            downwind_m=1000.0,
            crosswind_m=0.0,
            source_height_m=0.0,
            receptor_height_m=0.0,
            coeffs=COEFFS,
        )
        inert = concentration(decay_rate=0.0, **common)
        decayed = concentration(decay_rate=1e-4, **common)
        self.assertAlmostEqual(decayed / inert, math.exp(-1e-4 * 1000.0 / 2.0))


if __name__ == "__main__":
    unittest.main()
