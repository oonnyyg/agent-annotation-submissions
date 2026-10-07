import math
import unittest

from gapped_allan.core import (
    GridError,
    analyze,
    overlapping_allan,
    split_segments,
)


class GridTests(unittest.TestCase):
    def test_rejects_non_increasing_times(self):
        with self.assertRaises(GridError):
            split_segments([0.0, 2.0, 1.0], [0.0, 0.0, 0.0], 1.0)

    def test_rejects_off_grid_times(self):
        with self.assertRaises(GridError):
            split_segments([0.0, 1.5], [0.0, 0.0], 1.0)

    def test_rejects_bad_tau0(self):
        with self.assertRaises(GridError):
            split_segments([0.0, 1.0], [0.0, 0.0], 0.0)

    def test_gap_splits_segments(self):
        times = [0.0, 1.0, 2.0, 5.0, 6.0]
        segs = split_segments(times, [0.0] * 5, 1.0)
        self.assertEqual([s.size for s in segs], [3, 2])
        self.assertEqual(segs[1].start_index, 5)


class AllanTests(unittest.TestCase):
    def test_constant_offset_zero_deviation(self):
        times = [float(i) for i in range(20)]
        values = [1e-9] * 20
        result = analyze(times, values, 1.0, [1, 2, 4])
        for s in result.scales:
            self.assertEqual(s.status, "ok")
            self.assertAlmostEqual(s.deviation, 0.0, places=15)

    def test_linear_drift_scaling(self):
        # y = d*t: adjacent length-m means differ by d*m*tau0 exactly.
        d = 1e-9
        tau0 = 2.0
        times = [i * tau0 for i in range(40)]
        values = [d * t for t in times]
        result = analyze(times, values, tau0, [1, 2, 5])
        for s in result.scales:
            expected = d * s.m * tau0 / math.sqrt(2.0)
            self.assertAlmostEqual(s.deviation, expected, places=18)

    def test_detrend_removes_linear_drift(self):
        d = 3e-9
        times = [float(i) for i in range(30)]
        values = [d * t + 1e-8 for t in times]
        result = analyze(times, values, 1.0, [1, 2, 4], detrend=True)
        self.assertTrue(result.detrended)
        self.assertAlmostEqual(result.fit["slope"], d, places=18)
        self.assertAlmostEqual(result.fit["intercept"], 1e-8, places=18)
        for s in result.scales:
            self.assertAlmostEqual(s.deviation, 0.0, places=18)
        # Raw (pre-detrend) results are preserved and nonzero.
        for s in result.raw_scales:
            self.assertGreater(s.deviation, 0.0)

    def test_windows_never_cross_gap(self):
        # Two segments of 4 samples each, m=2 -> 1 window per segment.
        times = [0.0, 1.0, 2.0, 3.0, 10.0, 11.0, 12.0, 13.0]
        values = [0.0, 0.0, 1.0, 1.0, 5.0, 5.0, 9.0, 9.0]
        segs = split_segments(times, values, 1.0)
        res = overlapping_allan(segs, 2, 1.0)
        self.assertEqual(res.windows, 2)
        # Each segment: means (0,1) and (5,9) -> diffs 1 and 4.
        expected_var = (1.0**2 + 4.0**2) / (2 * 2)
        self.assertAlmostEqual(res.variance, expected_var)
        self.assertAlmostEqual(res.deviation, math.sqrt(expected_var))

    def test_pooled_not_average_of_deviations(self):
        # Pooling by window count differs from averaging per-segment devs.
        seg_long = [0.0, 1.0, 0.0, 1.0, 0.0, 1.0, 0.0, 1.0]
        seg_short = [2.0, 2.0, 0.0, 0.0]
        times = list(range(8)) + [20.0, 21.0, 22.0, 23.0]
        segs = split_segments(
            [float(t) for t in times], seg_long + seg_short, 1.0
        )
        pooled = overlapping_allan(segs, 2, 1.0)
        a = overlapping_allan(segs[:1], 2, 1.0)
        b = overlapping_allan(segs[1:], 2, 1.0)
        naive = (a.deviation + b.deviation) / 2
        self.assertNotAlmostEqual(pooled.deviation, naive)
        # Pooled variance is the window-count-weighted mean of variances.
        expected_var = (a.variance * a.windows + b.variance * b.windows) / (
            a.windows + b.windows
        )
        self.assertAlmostEqual(pooled.variance, expected_var)

    def test_insufficient_data_status(self):
        times = [float(i) for i in range(5)]
        result = analyze(times, [0.0] * 5, 1.0, [1, 3, 10])
        by_m = {s.m: s for s in result.scales}
        self.assertEqual(by_m[1].status, "ok")
        self.assertEqual(by_m[3].status, "insufficient_data")
        self.assertIsNone(by_m[3].deviation)
        self.assertEqual(by_m[10].status, "insufficient_data")

    def test_scales_sorted_by_tau(self):
        times = [2.0 * i for i in range(30)]
        result = analyze(times, [0.0] * 30, 2.0, [5, 1, 3])
        taus = [s.tau for s in result.scales]
        self.assertEqual(taus, sorted(taus))
        self.assertEqual([s.m for s in result.scales], [1, 3, 5])

    def test_gap_time_distance_preserved_in_fit(self):
        # A gap must not compress time: fit slope uses real timestamps.
        times = [0.0, 1.0, 2.0, 100.0, 101.0, 102.0]
        values = [2.0 * t for t in times]
        result = analyze(times, values, 1.0, [1], detrend=True)
        self.assertAlmostEqual(result.fit["slope"], 2.0, places=12)


if __name__ == "__main__":
    unittest.main()
