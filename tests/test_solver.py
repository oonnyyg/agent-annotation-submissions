import math
import unittest

from pseudorange_lab.loo import leave_one_out
from pseudorange_lab.solver import (
    ConvergenceError,
    InitialGeometryError,
    RankDeficientError,
    SatelliteObs,
    solve_epoch,
)

RX = (1_000_000.0, -2_000_000.0, 5_000_000.0)
CLOCK = 12_345.0

SAT_POSITIONS = [
    (20_000_000.0, 5_000_000.0, 15_000_000.0),
    (-12_000_000.0, 18_000_000.0, 14_000_000.0),
    (8_000_000.0, -20_000_000.0, 13_000_000.0),
    (-15_000_000.0, -10_000_000.0, 20_000_000.0),
    (3_000_000.0, 12_000_000.0, 22_000_000.0),
]


def make_sats(clock=CLOCK, sigmas=None, extra_range=None):
    sats = []
    for i, pos in enumerate(SAT_POSITIONS):
        rng = math.dist(RX, pos)
        bias = 0.0 if extra_range is None else extra_range[i]
        sats.append(SatelliteObs(
            sat_id=f"G{i + 1:02d}", x=pos[0], y=pos[1], z=pos[2],
            pseudorange=rng + clock + bias,
            sigma=1.0 if sigmas is None else sigmas[i]))
    return sats


class SolveEpochTests(unittest.TestCase):
    def test_recovers_known_position_and_clock(self):
        sol = solve_epoch(make_sats(), (0.0, 0.0, 0.0, 0.0))
        self.assertAlmostEqual(sol.x, RX[0], places=3)
        self.assertAlmostEqual(sol.y, RX[1], places=3)
        self.assertAlmostEqual(sol.z, RX[2], places=3)
        self.assertAlmostEqual(sol.clock, CLOCK, places=3)
        self.assertLess(sol.weighted_residual_sum_squares, 1e-12)
        for resid in sol.residuals.values():
            self.assertAlmostEqual(resid, 0.0, places=4)

    def test_common_clock_estimated_jointly_not_fixed(self):
        sol = solve_epoch(make_sats(clock=-777.0), (0.0, 0.0, 0.0, 0.0))
        self.assertAlmostEqual(sol.clock, -777.0, places=3)
        self.assertAlmostEqual(sol.x, RX[0], places=3)

    def test_weighting_uses_inverse_variance(self):
        noisy = [0.0, 0.0, 0.0, 0.0, 5.0]
        tight = make_sats(extra_range=noisy, sigmas=[1, 1, 1, 1, 100])
        loose = make_sats(extra_range=noisy, sigmas=[1, 1, 1, 1, 0.01])
        sol_tight = solve_epoch(tight, (0.0, 0.0, 0.0, 0.0))
        sol_loose = solve_epoch(loose, (0.0, 0.0, 0.0, 0.0))
        shift_tight = math.dist(RX, sol_tight.position)
        shift_loose = math.dist(RX, sol_loose.position)
        self.assertLess(shift_tight, shift_loose)
        self.assertLess(shift_tight, 1.0)

    def test_covariance_is_inverse_of_weighted_normal_matrix(self):
        sol = solve_epoch(make_sats(sigmas=[2.0] * 5), (0.0, 0.0, 0.0, 0.0))
        sol_unit = solve_epoch(make_sats(sigmas=[1.0] * 5), (0.0, 0.0, 0.0, 0.0))
        for i in range(4):
            self.assertAlmostEqual(
                sol.covariance[i][i], 4.0 * sol_unit.covariance[i][i], places=6)
            self.assertGreater(sol.covariance[i][i], 0.0)

    def test_degenerate_geometry_raises(self):
        base = SAT_POSITIONS[0]
        direction = (1.0, 2.0, 3.0)
        norm = math.sqrt(sum(v * v for v in direction))
        unit = tuple(v / norm for v in direction)
        sats = []
        for i, dist in enumerate((20e6, 22e6, 25e6, 30e6)):
            pos = tuple(base[k] + dist * unit[k] for k in range(3))
            sats.append(SatelliteObs(
                sat_id=f"G{i}", x=pos[0], y=pos[1], z=pos[2],
                pseudorange=math.dist(RX, pos) + CLOCK, sigma=1.0))
        with self.assertRaises(RankDeficientError):
            solve_epoch(sats, (0.0, 0.0, 0.0, 0.0))

    def test_initial_on_satellite_raises(self):
        pos = SAT_POSITIONS[0]
        with self.assertRaises(InitialGeometryError):
            solve_epoch(make_sats(), (pos[0], pos[1], pos[2], 0.0))

    def test_too_few_satellites_raises(self):
        with self.assertRaises(InitialGeometryError):
            solve_epoch(make_sats()[:3], (0.0, 0.0, 0.0, 0.0))

    def test_nonconvergence_raises(self):
        with self.assertRaises(ConvergenceError):
            solve_epoch(make_sats(), (0.0, 0.0, 0.0, 0.0), max_iterations=1)

    def test_duplicate_ids_rejected(self):
        sats = make_sats()
        sats.append(SatelliteObs(
            sat_id=sats[0].sat_id, x=1.0, y=2.0, z=3.0,
            pseudorange=2.0e7, sigma=1.0))
        with self.assertRaises(InitialGeometryError):
            solve_epoch(sats, (0.0, 0.0, 0.0, 0.0))


class LeaveOneOutTests(unittest.TestCase):
    def test_flags_outlier_via_extrapolated_residual(self):
        biases = [0.0, 0.0, 0.0, 0.0, 500.0]
        sats = make_sats(extra_range=biases)
        full = solve_epoch(sats, (0.0, 0.0, 0.0, 0.0))
        results = leave_one_out(sats, (0.0, 0.0, 0.0, 0.0), full)
        self.assertEqual(len(results), 5)
        self.assertTrue(all(r.ok for r in results))
        by_id = {r.excluded_sat_id: r for r in results}
        self.assertAlmostEqual(
            by_id["G05"].extrapolated_residual_m, 500.0, delta=1.0)
        clean = solve_epoch(sats[:4], (0.0, 0.0, 0.0, 0.0))
        self.assertAlmostEqual(clean.clock, CLOCK, places=3)
        self.assertAlmostEqual(clean.x, RX[0], places=3)

    def test_skipped_with_four_satellites(self):
        sats = make_sats()[:4]
        full = solve_epoch(sats, (0.0, 0.0, 0.0, 0.0))
        self.assertEqual(leave_one_out(sats, (0.0, 0.0, 0.0, 0.0), full), [])


if __name__ == "__main__":
    unittest.main()
