import unittest

from rainflow_fatigue.rainflow import count_cycles
from rainflow_fatigue.turning_points import Point, extract_turning_points


def count(stresses):
    points = [Point(float(i), s) for i, s in enumerate(stresses)]
    return count_cycles(extract_turning_points(points))


class RainflowTest(unittest.TestCase):
    def test_constant_record_no_cycles(self):
        self.assertEqual(count([5.0, 5.0, 5.0]), [])

    def test_nested_oscillation_closes_inner_full_cycle(self):
        # Outer excursion with a nested small oscillation.
        cycles = count([0.0, 10.0, 4.0, 6.0, 4.0, 10.0, 0.0])
        full = [c for c in cycles if c.count == 1.0]
        half = [c for c in cycles if c.count == 0.5]
        self.assertEqual(len(full), 2)
        self.assertAlmostEqual(full[0].stress_range, 2.0)
        self.assertAlmostEqual(full[0].mean, 5.0)
        self.assertAlmostEqual(full[1].stress_range, 6.0)
        self.assertAlmostEqual(sum(c.count for c in half), 1.0)
        self.assertAlmostEqual(sum(c.count for c in cycles), 3.0)

    def test_residual_half_cycles_kept_not_doubled(self):
        cycles = count([0.0, 8.0, 0.0, 6.0])
        self.assertTrue(all(c.count == 0.5 for c in cycles))
        self.assertAlmostEqual(sum(c.count for c in cycles), 1.5)
        self.assertEqual([c.stress_range for c in cycles], [8.0, 8.0, 6.0])

    def test_provenance_times_preserved(self):
        points = [Point(0.0, 0.0), Point(1.5, 4.0), Point(2.5, 0.0)]
        cycles = count_cycles(extract_turning_points(points))
        self.assertEqual([(c.start_time, c.end_time) for c in cycles],
                         [(0.0, 1.5), (1.5, 2.5)])

    def test_plateau_then_reversal(self):
        cycles = count([0.0, 3.0, 3.0, 3.0, 0.0])
        self.assertAlmostEqual(sum(c.count for c in cycles), 1.0)
        self.assertTrue(all(c.stress_range == 3.0 for c in cycles))


if __name__ == "__main__":
    unittest.main()
