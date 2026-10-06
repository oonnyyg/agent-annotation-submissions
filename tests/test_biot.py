import math
import unittest

from wirefield.biot import MU0_OVER_4PI, point_segment_distance, segment_field


class SegmentFieldTests(unittest.TestCase):
    def test_matches_infinite_wire_close_to_long_segment(self):
        # Point 0.01 m from the midpoint of a 200 m segment along x,
        # current +x: field ~ mu0*I/(2*pi*R) in +z... check direction:
        # dl=(+x), r from wire to point = +y, dl x r = +z.
        current = 3.0
        r = 0.01
        field = segment_field((0.0, r, 0.0), (-100.0, 0.0, 0.0), (100.0, 0.0, 0.0), current)
        expected = 2 * MU0_OVER_4PI * current / r  # mu0*I/(2*pi*R)
        self.assertAlmostEqual(field[2], expected, delta=expected * 1e-6)
        self.assertAlmostEqual(field[0], 0.0, places=15)
        self.assertAlmostEqual(field[1], 0.0, places=15)

    def test_point_on_extension_line_gives_zero(self):
        # Collinear but beyond the segment end: NOT an infinite wire.
        field = segment_field((5.0, 0.0, 0.0), (0.0, 0.0, 0.0), (1.0, 0.0, 0.0), 10.0)
        self.assertEqual(field, (0.0, 0.0, 0.0))

    def test_reversed_current_negates_field(self):
        a, b, p = (0.0, 0.0, 0.0), (1.0, 0.0, 0.0), (0.5, 0.5, 0.0)
        forward = segment_field(p, a, b, 2.0)
        backward = segment_field(p, a, b, -2.0)
        for f, r in zip(forward, backward):
            self.assertAlmostEqual(f, -r, places=18)

    def test_two_opposite_identical_coils_cancel(self):
        from wirefield.model import Coil
        from wirefield.solver import evaluate

        verts = ((1.0, 0.0, 0.0), (0.0, 1.0, 0.0), (-1.0, 0.0, 0.0), (0.0, -1.0, 0.0))
        coils = [Coil("a", verts, 5.0), Coil("b", verts, -5.0)]
        result = evaluate(coils, [5.0, -5.0], (0.0, 0.0, 2.0), 1e-9)
        self.assertTrue(result.computable)
        for component in result.total:
            self.assertAlmostEqual(component, 0.0, places=15)

    def test_point_segment_distance(self):
        self.assertAlmostEqual(
            point_segment_distance((0.5, 1.0, 0.0), (0.0, 0.0, 0.0), (1.0, 0.0, 0.0)), 1.0
        )
        self.assertAlmostEqual(
            point_segment_distance((2.0, 0.0, 0.0), (0.0, 0.0, 0.0), (1.0, 0.0, 0.0)), 1.0
        )


if __name__ == "__main__":
    unittest.main()
