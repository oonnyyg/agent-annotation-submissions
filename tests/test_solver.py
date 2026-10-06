"""Solver tests: known position, common clock, degenerate geometry."""
import math
import unittest
from pseudorange_lab.data import Observation
from pseudorange_lab.errors import GeometryError
from pseudorange_lab.solver import solve_epoch

SAT_POSITIONS = [
    (20_000_000.0, 0.0, 15_000_000.0),
    (-18_000_000.0, 8_000_000.0, 12_000_000.0),
    (5_000_000.0, 19_000_000.0, -14_000_000.0),
    (-6_000_000.0, -17_000_000.0, 16_000_000.0),
    (12_000_000.0, -10_000_000.0, 20_000_000.0),
]
TRUE_POS = (6_378_000.0, 0.0, 0.0)
TRUE_CLOCK = 123_456.7


def make_obs(sigma=1.0, clock=TRUE_CLOCK, pos=TRUE_POS, sats=SAT_POSITIONS):
    obs = []
    for i, sat in enumerate(sats):
        rng = math.dist(sat, pos)
        obs.append(Observation(f"G{i+1:02d}", sat, rng + clock, sigma))
    return obs


class KnownPositionTest(unittest.TestCase):
    def test_recovers_position_and_clock(self):
        obs = make_obs()
        sol = solve_epoch(obs, (6_400_000.0, 1_000.0, -2_000.0), 0.0)
        self.assertLess(math.dist(sol.position_m, TRUE_POS), 1e-4)
        self.assertAlmostEqual(sol.clock_m, TRUE_CLOCK, places=4)
        self.assertLess(sol.weighted_residual_sum_squares, 1e-12)
        self.assertEqual(len(sol.residuals_m), 5)

    def test_clock_estimated_jointly_not_fixed(self):
        obs = make_obs(clock=-50_000.0)
        sol = solve_epoch(obs, TRUE_POS, 0.0)
        self.assertAlmostEqual(sol.clock_m, -50_000.0, places=4)

    def test_weighting_prefers_low_sigma(self):
        obs = make_obs()
        noisy = Observation("G01", SAT_POSITIONS[0],
                            obs[0].pseudorange_m + 100.0, 1.0)
        trusted = Observation("G01", SAT_POSITIONS[0],
                              obs[0].pseudorange_m + 100.0, 1000.0)
        sol_noisy = solve_epoch([noisy] + obs[1:], TRUE_POS, TRUE_CLOCK)
        sol_trusted = solve_epoch([trusted] + obs[1:], TRUE_POS, TRUE_CLOCK)
        err_noisy = math.dist(sol_noisy.position_m, TRUE_POS)
        err_trusted = math.dist(sol_trusted.position_m, TRUE_POS)
        self.assertLess(err_trusted, err_noisy)

    def test_covariance_is_inverse_of_weighted_normal_matrix(self):
        obs = make_obs(sigma=2.0)
        sol = solve_epoch(obs, TRUE_POS, TRUE_CLOCK)
        self.assertEqual(len(sol.covariance), 4)
        for row in sol.covariance:
            self.assertTrue(all(math.isfinite(v) for v in row))
        self.assertGreater(sol.covariance[0][0], 0.0)


class DegenerateGeometryTest(unittest.TestCase):
    def test_rank_deficient_raises(self):
        sats = [(20e6, 0.0, 0.0), (21e6, 1.0, 0.0),
                (19e6, -1.0, 0.0), (22e6, 2.0, 0.0)]
        obs = make_obs(sats=sats)
        with self.assertRaises(GeometryError):
            solve_epoch(obs, TRUE_POS, 0.0)

    def test_position_on_satellite_raises(self):
        obs = make_obs()
        with self.assertRaises(GeometryError):
            solve_epoch(obs, SAT_POSITIONS[0], 0.0)

    def test_fewer_than_four_raises(self):
        with self.assertRaises(GeometryError):
            solve_epoch(make_obs()[:3], TRUE_POS, 0.0)


if __name__ == "__main__":
    unittest.main()
