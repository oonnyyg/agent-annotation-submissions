"""Tests for turning-point extraction and rainflow counting."""
from __future__ import annotations

import unittest

from rainflow_audit.rainflow import count_cycles
from rainflow_audit.turning_points import Point, extract_turning_points


class TestTurningPoints(unittest.TestCase):
    def test_constant_record_yields_zero_cycles(self):
        points = [Point(0.0, 5.0), Point(1.0, 5.0), Point(2.0, 5.0)]
        turning = extract_turning_points(points)
        self.assertEqual(len(turning), 1)
        self.assertEqual(count_cycles(turning), [])

    def test_plateau_keeps_last_point(self):
        points = [
            Point(0.0, 0.0),
            Point(1.0, 5.0),
            Point(2.0, 5.0),
            Point(3.0, 5.0),
            Point(4.0, 1.0),
            Point(5.0, 8.0),
            Point(6.0, 2.0),
            Point(7.0, 6.0),
            Point(8.0, 0.0),
        ]
        turning = extract_turning_points(points)
        stresses = [p.stress for p in turning]
        self.assertEqual(stresses, [0.0, 5.0, 1.0, 8.0, 2.0, 6.0, 0.0])
        self.assertEqual(turning[1].time, 3.0)

    def test_plateau_at_reversal_keeps_last(self):
        points = [Point(0.0, 0.0), Point(1.0, 5.0), Point(2.0, 5.0), Point(3.0, -1.0)]
        turning = extract_turning_points(points)
        self.assertEqual([(p.time, p.stress) for p in turning],
                         [(0.0, 0.0), (2.0, 5.0), (3.0, -1.0)])


class TestRainflow(unittest.TestCase):
    def test_residual_half_cycles_kept(self):
        cycles = count_cycles([Point(0.0, 0.0), Point(1.0, 5.0), Point(2.0, -5.0)])
        self.assertEqual(len(cycles), 2)
        self.assertEqual(cycles[0].count, 0.5)
        self.assertEqual(cycles[0].stress_range, 5.0)
        self.assertEqual(cycles[1].count, 0.5)
        self.assertEqual(cycles[1].stress_range, 10.0)
        self.assertAlmostEqual(sum(c.count for c in cycles), 1.0)

    def test_nested_waves(self):
        turning = extract_turning_points([
            Point(0.0, 0.0), Point(1.0, 10.0), Point(2.0, -5.0),
            Point(3.0, 5.0), Point(4.0, -10.0), Point(5.0, 0.0),
        ])
        cycles = count_cycles(turning)
        full = [c for c in cycles if c.count == 1.0]
        half = [c for c in cycles if c.count == 0.5]
        self.assertEqual(len(full), 1)
        self.assertEqual(full[0].stress_range, 10.0)
        self.assertEqual(full[0].mean, 0.0)
        self.assertEqual(len(half), 3)
        self.assertEqual(sorted(c.stress_range for c in half), [10.0, 10.0, 20.0])
        self.assertAlmostEqual(sum(c.count for c in cycles), 2.5)

    def test_provenance_times(self):
        cycles = count_cycles([Point(0.0, 0.0), Point(1.0, 5.0), Point(2.0, -5.0)])
        self.assertEqual((cycles[0].start_time, cycles[0].end_time), (0.0, 1.0))
        self.assertEqual((cycles[1].start_time, cycles[1].end_time), (1.0, 2.0))


if __name__ == "__main__":
    unittest.main()
