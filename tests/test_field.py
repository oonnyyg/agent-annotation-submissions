import math
import unittest

from wirefield.coil import Coil, CoilDataError, load_config, parse_coil
from wirefield.field import (
    MU0,
    coil_field,
    coil_min_distance,
    field_per_coil,
    segment_field,
    total_field,
)
from wirefield.vector import norm


def square(side, current, z=0.0):
    h = side / 2.0
    return Coil(
        "sq",
        ((-h, -h, z), (h, -h, z), (h, h, z), (-h, h, z)),
        current,
    )


class SegmentFieldTest(unittest.TestCase):
    def test_long_segment_matches_infinite_wire(self):
        got = segment_field((1.0, 0.0, 0.0), (0, 0, -1e6), (0, 0, 1e6), 2.0)
        expected = MU0 * 2.0 / (2.0 * math.pi * 1.0)
        self.assertAlmostEqual(got[1], expected, delta=expected * 1e-9)
        self.assertAlmostEqual(got[0], 0.0)
        self.assertAlmostEqual(got[2], 0.0)

    def test_collinear_point_inside_segment_is_zero(self):
        self.assertEqual(
            segment_field((0.0, 0.0, 0.5), (0, 0, 0), (0, 0, 1), 5.0), (0, 0, 0)
        )

    def test_point_on_extension_line_is_zero_not_infinite(self):
        got = segment_field((0.0, 0.0, 10.0), (0, 0, 0), (0, 0, 1), 5.0)
        self.assertEqual(got, (0.0, 0.0, 0.0))

    def test_direction_follows_right_hand_rule(self):
        got = segment_field((1.0, 0.0, 0.0), (0, 0, -1), (0, 0, 1), 1.0)
        self.assertGreater(got[1], 0.0)
        got_rev = segment_field((1.0, 0.0, 0.0), (0, 0, -1), (0, 0, 1), -1.0)
        self.assertLess(got_rev[1], 0.0)


class CoilFieldTest(unittest.TestCase):
    def test_square_loop_center(self):
        loop = square(2.0, 1.0)
        got = coil_field(loop, (0.0, 0.0, 0.0))
        expected = 2.0 * math.sqrt(2.0) * MU0 * 1.0 / (math.pi * 2.0)
        self.assertAlmostEqual(got[2], expected, delta=expected * 1e-12)

    def test_reversed_currents_cancel(self):
        a = Coil("a", square(2.0, 1.0).vertices, 3.0)
        b = Coil("b", square(2.0, 1.0).vertices, -3.0)
        per_coil = field_per_coil([a, b], (0.3, -0.2, 0.7))
        total = total_field(per_coil)
        self.assertAlmostEqual(norm(total), 0.0, places=15)
        self.assertAlmostEqual(norm(per_coil["a"]), norm(per_coil["b"]), places=15)

    def test_current_scaling_scales_field(self):
        loop = square(2.0, 1.0)
        p = (0.1, 0.2, 0.3)
        b1 = coil_field(loop, p, 1.0)
        b2 = coil_field(loop, p, -2.5)
        for i in range(3):
            self.assertAlmostEqual(b2[i], -2.5 * b1[i], places=18)

    def test_min_distance(self):
        loop = square(2.0, 1.0)
        self.assertAlmostEqual(coil_min_distance(loop, (0.0, 0.0, 0.0)), 1.0)
        self.assertAlmostEqual(
            coil_min_distance(loop, (0.0, 0.0, 0.5)), math.sqrt(1.25)
        )


class CoilValidationTest(unittest.TestCase):
    def base(self, **overrides):
        data = {
            "id": "c",
            "vertices": [[0, 0, 0], [1, 0, 0], [0, 1, 0]],
            "current": 1.0,
        }
        data.update(overrides)
        return data

    def test_valid_triangle(self):
        coil = parse_coil(self.base(), 0)
        self.assertEqual(len(coil.segments()), 3)

    def test_rejects_two_vertices(self):
        with self.assertRaises(CoilDataError):
            parse_coil(self.base(vertices=[[0, 0, 0], [1, 0, 0]]), 0)

    def test_rejects_zero_length_edge(self):
        with self.assertRaises(CoilDataError):
            parse_coil(self.base(vertices=[[0, 0, 0], [0, 0, 0], [1, 0, 0]]), 0)

    def test_rejects_zero_length_closing_edge(self):
        with self.assertRaises(CoilDataError):
            parse_coil(
                self.base(vertices=[[0, 0, 0], [1, 0, 0], [0, 1, 0], [0, 0, 0]]), 0
            )

    def test_rejects_non_finite(self):
        with self.assertRaises(CoilDataError):
            parse_coil(self.base(vertices=[[0, 0, 0], [1, 0, 0], [0, 1, "nan"]]), 0)
        with self.assertRaises(CoilDataError):
            parse_coil(self.base(current=float("inf")), 0)

    def test_scenario_requires_all_coils(self):
        config = {
            "coils": [self.base(), self.base(id="d")],
            "scenarios": [{"name": "s", "currents": {"c": 2.0}}],
        }
        with self.assertRaises(CoilDataError):
            load_config(config)

    def test_scenario_unknown_coil_rejected(self):
        config = {
            "coils": [self.base()],
            "scenarios": [{"name": "s", "currents": {"c": 2.0, "nope": 1.0}}],
        }
        with self.assertRaises(CoilDataError):
            load_config(config)


if __name__ == "__main__":
    unittest.main()
