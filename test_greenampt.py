import math
import unittest

from greenampt import (Segment, SoilParams, SolverError, cumulative_time,
                       infiltration_capacity, ponding_cumulative, simulate,
                       solve_cumulative, totals)

SOIL = SoilParams(ks=10.0, psi=200.0, delta_theta=0.3)  # M = 60 mm


class TestParams(unittest.TestCase):
    def test_invalid_params_rejected(self):
        for kw in (dict(ks=0, psi=1, delta_theta=0.5),
                   dict(ks=-1, psi=1, delta_theta=0.5),
                   dict(ks=1, psi=0, delta_theta=0.5),
                   dict(ks=1, psi=1, delta_theta=0),
                   dict(ks=1, psi=1, delta_theta=1.5)):
            with self.assertRaises(ValueError):
                SoilParams(**kw)

    def test_capacity_limit_at_zero(self):
        self.assertEqual(infiltration_capacity(SOIL, 0.0), float("inf"))
        self.assertAlmostEqual(infiltration_capacity(SOIL, 60.0), 20.0)


class TestSolver(unittest.TestCase):
    def test_roundtrip(self):
        for f in (1.0, 25.0, 250.0):
            t = cumulative_time(SOIL, f, 0.0, 0.0)
            self.assertAlmostEqual(
                solve_cumulative(SOIL, t, 0.0, 0.0), f, places=6)

    def test_no_convergence_raises(self):
        with self.assertRaises(SolverError):
            solve_cumulative(SOIL, 5.0, 0.0, 0.0, tol=1e-15, max_iter=2)


class TestSimulate(unittest.TestCase):
    def test_light_rain_all_infiltrates(self):
        segs = [Segment(0, 2, 5.0)]
        events = simulate(segs, SOIL)
        rain, infil, runoff = totals(segs, events)
        self.assertAlmostEqual(infil, 10.0)
        self.assertAlmostEqual(runoff, 0.0)

    def test_ponding_inside_segment(self):
        # F_p = Ks*M/(i-Ks) = 10*60/30 = 20 mm; at 40 mm/h ponding at 0.5 h.
        segs = [Segment(0, 2, 40.0)]
        events = simulate(segs, SOIL)
        pond = [e for e in events if e.kind == "ponding"]
        self.assertEqual(len(pond), 1)
        self.assertAlmostEqual(pond[0].t, 0.5)
        self.assertAlmostEqual(pond[0].cum_infiltration, 20.0)
        rain, infil, runoff = totals(segs, events)
        self.assertAlmostEqual(rain, infil + runoff)
        self.assertGreater(runoff, 0.0)

    def test_dry_period_keeps_cumulative(self):
        segs = [Segment(0, 1, 5.0), Segment(1, 3, 0.0), Segment(3, 4, 5.0)]
        events = simulate(segs, SOIL)
        at3 = [e for e in events if e.t == 3.0][0]
        self.assertAlmostEqual(at3.cum_infiltration, 5.0)
        rain, infil, runoff = totals(segs, events)
        self.assertAlmostEqual(infil, 10.0)

    def test_split_segment_consistency(self):
        coarse = [Segment(0, 3, 40.0)]
        fine = [Segment(h, h + 0.5, 40.0) for h in
                [x * 0.5 for x in range(6)]]
        e1 = simulate(coarse, SOIL)[-1]
        e2 = simulate(fine, SOIL)[-1]
        self.assertAlmostEqual(e1.cum_infiltration, e2.cum_infiltration,
                               places=6)
        self.assertAlmostEqual(e1.cum_runoff, e2.cum_runoff, places=6)

    def test_validation(self):
        with self.assertRaises(ValueError):  # gap
            simulate([Segment(0, 1, 5), Segment(2, 3, 5)], SOIL)
        with self.assertRaises(ValueError):  # overlap
            simulate([Segment(0, 2, 5), Segment(1, 3, 5)], SOIL)
        with self.assertRaises(ValueError):  # negative intensity
            simulate([Segment(0, 1, -1)], SOIL)
        with self.assertRaises(ValueError):  # zero duration
            simulate([Segment(0, 0, 5)], SOIL)

    def test_water_balance_mixed_storm(self):
        segs = [Segment(0, 1, 8.0), Segment(1, 2, 50.0),
                Segment(2, 4, 0.0), Segment(4, 5, 30.0)]
        events = simulate(segs, SOIL)
        rain, infil, runoff = totals(segs, events)
        self.assertAlmostEqual(rain, infil + runoff, places=9)


if __name__ == "__main__":
    unittest.main()
