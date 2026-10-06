import unittest

from hull_hydrostatics import geometry


def rect(w=2.0, h=1.0):
    return [(-w / 2, 0.0), (w / 2, 0.0), (w / 2, h), (-w / 2, h)]


class GeometryTests(unittest.TestCase):
    def test_signed_area_and_winding(self):
        r = rect()
        self.assertAlmostEqual(abs(geometry.polygon_signed_area(r)), 2.0)
        rev = list(reversed(r))
        self.assertAlmostEqual(geometry.polygon_area(rev), 2.0)
        self.assertAlmostEqual(geometry.polygon_centroid(rev)[0], 0.0)
        self.assertAlmostEqual(geometry.polygon_centroid(rev)[1], 0.5)

    def test_reject_self_intersection(self):
        bowtie = [(0, 0), (2, 2), (2, 0), (0, 2)]
        with self.assertRaises(ValueError):
            geometry.validate_simple_polygon(bowtie)

    def test_reject_zero_area(self):
        with self.assertRaises(ValueError):
            geometry.validate_simple_polygon([(0, 0), (1, 0), (2, 0)])

    def test_concave_polygon_accepted(self):
        u_shape = [(0, 0), (3, 0), (3, 2), (2, 2), (2, 1), (1, 1),
                   (1, 2), (0, 2)]
        geometry.validate_simple_polygon(u_shape)
        self.assertAlmostEqual(geometry.polygon_area(u_shape), 5.0)

    def test_clip_concave_waterline(self):
        # Arch shape (notch opening downward) clipped at y=0.5 gives two
        # separate submerged legs, each 1x0.5.
        arch = [(0, 0), (1, 0), (1, 1), (2, 1), (2, 0), (3, 0),
                (3, 2), (0, 2)]
        geometry.validate_simple_polygon(arch)
        parts = geometry.clip_below_waterline(arch, 0.5)
        area = sum(geometry.polygon_area(p) for p in parts)
        self.assertAlmostEqual(area, 1.0)
        num = 0.0
        for p in parts:
            num += geometry.polygon_area(p) * geometry.polygon_centroid(p)[0]
        self.assertAlmostEqual(num / area, 1.5)


if __name__ == "__main__":
    unittest.main()
