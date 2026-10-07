import math
import unittest

from gapped_allan.core import (
    AnalysisError,
    build_segments,
    detrend_ols,
    overlapping_allan,
)


class TestGrid(unittest.TestCase):
    def test_strictly_increasing_required(self):
        with self.assertRaises(AnalysisError):
            build_segments([0.0, 1.0, 1.0], [0.0, 0.0, 0.0], 1.0)

    def test_off_grid_rejected(self):
        with self.assertRaises(AnalysisError):
            build_segments([0.0, 1.0, 2.5], [0.0, 0.0, 0.0], 1.0)

    def test_gap_splits_segments(self):
        times = [0.0, 1.0, 2.0, 5.0, 6.0]
        segs = build_segments(times, [0.0] * 5, 1.0)
        self.assertEqual([s.size for s in segs], [3, 2])
        self.assertEqual(segs[1].start_index, 5)

    def test_noninteger_tau0_grid(self):
        times = [0.0, 0.1, 0.2, 0.3]
        segs = build_segments(times, [1.0] * 4, 0.1)
        self.assertEqual(len(segs), 1)


class TestConstantOffset(unittest.TestCase):
    def test_constant_frequency_gives_zero_adev(self):
        n = 200
        times = [float(i) for i in range(n)]
        values = [1e-9] * n
        segs = build_segments(times, values, 1.0)
        results = overlapping_allan(segs, [1, 2, 4, 8], 1.0)
        for r in results:
            self.assertEqual(r.status, "ok")
            self.assertAlmostEqual(r.adev, 0.0, places=15)
        self.assertEqual(results[0].pairs, n - 1)

    def test_white_noise_matches_theory(self):
        # Deterministic +/- alternation: each diff is +-2, avar = 4/2 = 2.
        n = 101
        times = [float(i) for i in range(n)]
        values = [1.0 if i % 2 == 0 else -1.0 for i in range(n)]
        segs = build_segments(times, values, 1.0)
        r = overlapping_allan(segs, [1], 1.0)[0]
        self.assertAlmostEqual(r.avar, 2.0, places=12)

    def test_insufficient_data_status(self):
        times = [float(i) for i in range(5)]
        segs = build_segments(times, [0.0] * 5, 1.0)
        r = overlapping_allan(segs, [4], 1.0)[0]
        self.assertEqual(r.status, "insufficient_data")
        self.assertIsNone(r.adev)
        self.assertIsNone(r.avar)
        self.assertEqual(r.pairs, 0)


class TestSegmentMerging(unittest.TestCase):
    def test_pooled_by_pair_count(self):
        # Two segments: sizes 5 and 3, m=1 -> 4 + 2 = 6 pairs.
        times = [0.0, 1.0, 2.0, 3.0, 4.0, 10.0, 11.0, 12.0]
        values = [0.0, 1.0, 0.0, 1.0, 0.0, 2.0, 0.0, 2.0]
        segs = build_segments(times, values, 1.0)
        self.assertEqual(len(segs), 2)
        r = overlapping_allan(segs, [1], 1.0)[0]
        self.assertEqual(r.pairs, 6)
        # seg1 diffs: 1,-1,1,-1 -> sq 1 each; seg2 diffs: -2, 2 -> sq 4 each
        expected = (4 * 1.0 + 2 * 4.0) / (2 * 6)
        self.assertAlmostEqual(r.avar, expected, places=12)
        self.assertAlmostEqual(r.adev, math.sqrt(expected), places=12)

    def test_no_cross_gap_pairs(self):
        # If windows crossed the gap, pairs would be larger than sum.
        times = [0.0, 1.0, 2.0, 100.0, 101.0, 102.0]
        segs = build_segments(times, [0.0] * 6, 1.0)
        r = overlapping_allan(segs, [2], 1.0)[0]
        # each segment of 3 samples gives 3 - 4 + 1 = 0 pairs
        self.assertEqual(r.pairs, 0)
        self.assertEqual(r.status, "insufficient_data")


class TestDetrend(unittest.TestCase):
    def test_linear_drift_removed(self):
        n = 100
        times = [float(i) for i in range(n)]
        values = [2.0 * t + 5.0 for t in times]
        slope, intercept, residuals = detrend_ols(times, values)
        self.assertAlmostEqual(slope, 2.0, places=10)
        self.assertAlmostEqual(intercept, 5.0, places=10)
        segs = build_segments(times, residuals, 1.0)
        r = overlapping_allan(segs, [1], 1.0)[0]
        self.assertAlmostEqual(r.adev, 0.0, places=10)

    def test_fit_uses_real_times_across_gap(self):
        # Gap must not be compressed: fit on actual timestamps.
        times = [0.0, 1.0, 2.0, 100.0, 101.0, 102.0]
        values = [t for t in times]  # slope exactly 1 in real time
        slope, intercept, residuals = detrend_ols(times, values)
        self.assertAlmostEqual(slope, 1.0, places=12)
        self.assertAlmostEqual(intercept, 0.0, places=12)
        for res in residuals:
            self.assertAlmostEqual(res, 0.0, places=10)

    def test_linear_drift_adev_scaling(self):
        # For y = a*t, avar(m) = (a * m * tau0)^2 / 2.
        n = 200
        a = 3.0
        times = [float(i) for i in range(n)]
        values = [a * t for t in times]
        segs = build_segments(times, values, 1.0)
        for m in (1, 2, 5):
            r = overlapping_allan(segs, [m], 1.0)[0]
            expected = (a * m) ** 2 / 2.0
            self.assertAlmostEqual(r.avar, expected, places=8)


if __name__ == "__main__":
    unittest.main()
