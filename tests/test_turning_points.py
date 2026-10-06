import unittest

from rainflow_fatigue.turning_points import Point, extract_turning_points


def pts(stresses):
    return [Point(float(i), s) for i, s in enumerate(stresses)]


class TurningPointsTest(unittest.TestCase):
    def test_plateau_keeps_last_point(self):
        result = extract_turning_points(pts([0.0, 1.0, 1.0, 1.0, -1.0, 0.0]))
        self.assertEqual([p.stress for p in result], [0.0, 1.0, -1.0, 0.0])
        self.assertEqual(result[1].time, 3.0)  # last point of the plateau

    def test_constant_record_has_no_reversal(self):
        result = extract_turning_points(pts([2.0, 2.0, 2.0]))
        self.assertEqual(len(result), 1)

    def test_first_last_and_reversals_kept(self):
        result = extract_turning_points(pts([0.0, 2.0, 1.0, 3.0, 0.0]))
        self.assertEqual([p.stress for p in result], [0.0, 2.0, 1.0, 3.0, 0.0])

    def test_monotone_keeps_only_endpoints(self):
        result = extract_turning_points(pts([0.0, 1.0, 2.0, 3.0]))
        self.assertEqual([p.stress for p in result], [0.0, 3.0])


if __name__ == "__main__":
    unittest.main()
