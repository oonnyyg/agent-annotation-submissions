import math
import unittest

from green_ampt.model import (
    Segment,
    SoilParams,
    capacity,
    simulate,
    solve_cum_infiltration,
)

SOIL = SoilParams(ks=10.0, psi=160.0, delta_theta=0.3)


class TestSoilParams(unittest.TestCase):
    def test_rejects_non_positive_ks(self):
        with self.assertRaises(ValueError):
            SoilParams(ks=0.0, psi=160.0, delta_theta=0.3)

    def test_rejects_non_positive_psi(self):
        with self.assertRaises(ValueError):
            SoilParams(ks=10.0, psi=-1.0, delta_theta=0.3)

    def test_rejects_delta_theta_above_one(self):
        with self.assertRaises(ValueError):
            SoilParams(ks=10.0, psi=160.0, delta_theta=1.5)

    def test_rejects_zero_delta_theta(self):
        with self.assertRaises(ValueError):
            SoilParams(ks=10.0, psi=160.0, delta_theta=0.0)


class TestCapacity(unittest.TestCase):
    def test_zero_cum_infiltration_is_infinite_limit(self):
        self.assertEqual(capacity(SOIL, 0.0), math.inf)

    def test_capacity_formula(self):
        self.assertAlmostEqual(capacity(SOIL, 48.0), 10.0 * (1.0 + 48.0 / 48.0))

    def test_capacity_decreases_with_f(self):
        self.assertGreater(capacity(SOIL, 10.0), capacity(SOIL, 100.0))


class TestSolver(unittest.TestCase):
    def test_cumulative_relation_consistency(self):
        # F solved at t_end must satisfy the cumulative Green-Ampt relation
        t_end = 2.0
        f = solve_cum_infiltration(SOIL, 0.0, 0.0, t_end)
        m = SOIL.psi * SOIL.delta_theta
        t_check = (f - m * math.log((m + f) / m)) / SOIL.ks
        self.assertAlmostEqual(t_check, t_end, places=6)

    def test_zero_elapsed_time_returns_initial(self):
        self.assertEqual(solve_cum_infiltration(SOIL, 1.0, 5.0, 1.0), 5.0)

    def test_non_convergence_raises(self):
        with self.assertRaises(RuntimeError):
            solve_cum_infiltration(SOIL, 0.0, 0.0, 5.0, tol=1e-12, max_iter=3)


class TestSimulate(unittest.TestCase):
    def test_light_rain_all_infiltrates(self):
        res = simulate(SOIL, [Segment(0.0, 2.0, 5.0)])
        self.assertAlmostEqual(res.total_rain, 10.0)
        self.assertAlmostEqual(res.total_infiltration, 10.0)
        self.assertAlmostEqual(res.total_runoff, 0.0)

    def test_ponding_inside_segment(self):
        res = simulate(SOIL, [Segment(0.0, 3.0, 40.0)])
        kinds = [ev.kind for ev in res.events]
        self.assertIn("ponding", kinds)
        pond = next(ev for ev in res.events if ev.kind == "ponding")
        # Fp = M / (i/Ks - 1) = 48 / 3 = 16 mm at t = 16/40 = 0.4 h
        self.assertAlmostEqual(pond.time, 0.4, places=9)
        self.assertAlmostEqual(pond.cum_infiltration, 16.0, places=9)
        self.assertGreater(res.total_runoff, 0.0)
        self.assertAlmostEqual(
            res.total_rain, res.total_infiltration + res.total_runoff, places=9
        )

    def test_dry_period_preserves_state(self):
        res = simulate(
            SOIL,
            [Segment(0.0, 1.0, 40.0), Segment(1.0, 2.0, 0.0), Segment(2.0, 3.0, 40.0)],
        )
        wet_end = next(ev for ev in res.events if ev.time == 1.0)
        dry_end = next(ev for ev in res.events if ev.time == 2.0)
        self.assertAlmostEqual(dry_end.cum_infiltration, wet_end.cum_infiltration)
        self.assertAlmostEqual(dry_end.cum_rain, wet_end.cum_rain)
        # infiltration continues after the dry spell from the same state
        self.assertGreater(res.total_infiltration, dry_end.cum_infiltration)

    def test_split_segments_match_single_segment(self):
        single = simulate(SOIL, [Segment(0.0, 4.0, 40.0)])
        split = simulate(
            SOIL,
            [Segment(0.0, 1.0, 40.0), Segment(1.0, 2.5, 40.0), Segment(2.5, 4.0, 40.0)],
        )
        self.assertAlmostEqual(single.total_infiltration, split.total_infiltration, places=6)
        self.assertAlmostEqual(single.total_runoff, split.total_runoff, places=6)

    def test_mass_balance_closes(self):
        res = simulate(
            SOIL,
            [Segment(0.0, 1.0, 5.0), Segment(1.0, 3.0, 40.0), Segment(3.0, 4.0, 0.0)],
        )
        self.assertAlmostEqual(
            res.total_rain, res.total_infiltration + res.total_runoff, places=9
        )

    def test_immediate_ponding_when_rate_exceeds_capacity(self):
        # ponded state carries over: second heavy segment starts capacity-limited
        res = simulate(SOIL, [Segment(0.0, 1.0, 40.0), Segment(1.0, 2.0, 40.0)])
        kinds = [ev.kind for ev in res.events]
        self.assertEqual(kinds.count("ponding"), 1)


if __name__ == "__main__":
    unittest.main()