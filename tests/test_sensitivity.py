"""Leave-one-out sensitivity tests."""
import math
import unittest

from pseudorange_lab.sensitivity import leave_one_out
from pseudorange_lab.solver import solve_epoch
from test_solver import SAT_POSITIONS, TRUE_CLOCK, TRUE_POS, make_obs


class LeaveOneOutTest(unittest.TestCase):
    def test_clean_data_small_shifts(self):
        obs = make_obs()
        full = solve_epoch(obs, TRUE_POS, TRUE_CLOCK)
        results = leave_one_out(obs, TRUE_POS, TRUE_CLOCK, full)
        self.assertEqual(len(results), len(obs))
        for res in results:
            self.assertEqual(res.status, "ok")
            self.assertLess(res.position_shift_m, 1e-3)
            self.assertLess(abs(res.extrapolated_residual_m), 1e-3)

    def test_outlier_detected_by_extrapolated_residual(self):
        obs = make_obs()
        extra_pos = (-15e6, -12e6, 18e6)
        rng = math.dist(extra_pos, TRUE_POS)
        obs.append(type(obs[0])("G06", extra_pos, rng + TRUE_CLOCK, 1.0))
        bad = obs[2]
        obs[2] = type(bad)(bad.sat_id, bad.position_m,
                           bad.pseudorange_m + 500.0, bad.sigma_m)
        full = solve_epoch(obs, TRUE_POS, TRUE_CLOCK)
        results = leave_one_out(obs, TRUE_POS, TRUE_CLOCK, full)
        dropped_bad = next(r for r in results if r.excluded_sat_id == bad.sat_id)
        self.assertGreater(abs(dropped_bad.extrapolated_residual_m), 100.0)
        others = [r for r in results if r.excluded_sat_id != bad.sat_id]
        self.assertTrue(all(
            abs(r.extrapolated_residual_m) < abs(dropped_bad.extrapolated_residual_m)
            for r in others
        ))

    def test_degenerate_subset_flagged(self):
        base = make_obs()
        obs = []
        coplanar_positions = [
            (20e6, 5e6, 0.0),
            (-15e6, 10e6, 0.0),
            (5e6, -20e6, 0.0),
            (-10e6, -15e6, 0.0),
        ]
        for i, pos in enumerate(coplanar_positions):
            rng = math.dist(pos, TRUE_POS)
            obs.append(type(base[0])(f"C{i+1}", pos, rng + TRUE_CLOCK, 1.0))
        off_plane = (5e6, 5e6, 22e6)
        rng = math.dist(off_plane, TRUE_POS)
        obs.append(type(base[0])("OFF", off_plane, rng + TRUE_CLOCK, 1.0))
        full = solve_epoch(obs, TRUE_POS, TRUE_CLOCK)
        results = leave_one_out(obs, TRUE_POS, TRUE_CLOCK, full)
        flagged = [r for r in results if r.status == "degenerate"]
        self.assertEqual([r.excluded_sat_id for r in flagged], ["OFF"])
        for res in flagged:
            self.assertIsNone(res.position_shift_m)
            self.assertIsNotNone(res.detail)

    def test_four_satellites_returns_none(self):
        obs = make_obs()[:4]
        full = solve_epoch(obs, TRUE_POS, TRUE_CLOCK)
        self.assertIsNone(leave_one_out(obs, TRUE_POS, TRUE_CLOCK, full))


if __name__ == "__main__":
    unittest.main()
