import math
import unittest

from binding import Experiment, SiteClass, invert_dose, solve_equilibrium

SITES = [
    SiteClass("receptor", total=10.0, kd_a=1.0, kd_b=5.0),
    SiteClass("decoy", total=50.0, kd_a=100.0, kd_b=2.0),
]


class SolverTests(unittest.TestCase):
    def test_mass_conservation_and_bounds(self):
        exp = Experiment(20.0, 30.0)
        r = solve_equilibrium(SITES, exp)
        self.assertTrue(r.converged)
        self.assertGreaterEqual(r.free_a, 0.0)
        self.assertLessEqual(r.free_a, exp.total_a)
        self.assertGreaterEqual(r.free_b, 0.0)
        self.assertLessEqual(r.free_b, exp.total_b)
        self.assertAlmostEqual(r.free_a + r.bound_a, exp.total_a, places=9)
        self.assertAlmostEqual(r.free_b + r.bound_b, exp.total_b, places=9)
        for c in r.classes:
            self.assertAlmostEqual(c.frac_free + c.occ_a + c.occ_b, 1.0, 12)

    def test_competition_reduces_occupancy(self):
        alone = solve_equilibrium(SITES, Experiment(5.0, 0.0))
        competed = solve_equilibrium(SITES, Experiment(5.0, 100.0))
        for c_alone, c_comp in zip(alone.classes, competed.classes):
            self.assertLess(c_comp.occ_a, c_alone.occ_a)
        self.assertGreater(competed.free_a, alone.free_a)

    def test_zero_doses(self):
        r = solve_equilibrium(SITES, Experiment(0.0, 0.0))
        self.assertEqual(r.free_a, 0.0)
        self.assertEqual(r.free_b, 0.0)
        for c in r.classes:
            self.assertEqual(c.occ_a, 0.0)
            self.assertEqual(c.occ_b, 0.0)
            self.assertEqual(c.frac_free, 1.0)

    def test_zero_dose_of_one_ligand(self):
        r = solve_equilibrium(SITES, Experiment(0.0, 10.0))
        self.assertEqual(r.free_a, 0.0)
        self.assertGreater(r.free_b, 0.0)
        self.assertTrue(all(c.occ_a == 0.0 for c in r.classes))

    def test_free_is_not_total_when_sites_dense(self):
        dense = [SiteClass("dense", total=1000.0, kd_a=1.0, kd_b=1.0)]
        r = solve_equilibrium(dense, Experiment(10.0, 0.0))
        self.assertLess(r.free_a, 10.0)
        self.assertGreater(r.bound_a, 0.0)

    def test_single_class_matches_quadratic(self):
        s = [SiteClass("x", total=8.0, kd_a=2.0, kd_b=3.0)]
        t = 5.0
        r = solve_equilibrium(s, Experiment(t, 0.0))
        k = 8.0 + t + 2.0
        bound = 0.5 * (k - math.sqrt(k * k - 4 * 8.0 * t))
        self.assertAlmostEqual(r.bound_a, bound, places=9)

    def test_invalid_inputs_rejected(self):
        with self.assertRaises(ValueError):
            SiteClass("bad", total=0.0, kd_a=1.0, kd_b=1.0)
        with self.assertRaises(ValueError):
            SiteClass("bad", total=1.0, kd_a=-1.0, kd_b=1.0)
        with self.assertRaises(ValueError):
            SiteClass("bad", total=1.0, kd_a=1.0, kd_b=float("nan"))
        with self.assertRaises(ValueError):
            Experiment(-1.0, 0.0)
        with self.assertRaises(ValueError):
            Experiment(float("inf"), 0.0)


class InverseTests(unittest.TestCase):
    def test_inverse_recovers_target(self):
        res = invert_dose(SITES, "receptor", 0.6, total_b=20.0, limit_a=1e4)
        self.assertTrue(res.achieved)
        self.assertIsNotNone(res.total_a)
        self.assertLessEqual(res.total_a, 1e4)
        self.assertAlmostEqual(res.occupancy_at_solution, 0.6, places=6)

    def test_inverse_minimality(self):
        res = invert_dose(SITES, "receptor", 0.5, total_b=0.0, limit_a=1e4)
        slightly_less = solve_equilibrium(
            SITES, Experiment(res.total_a * 0.99, 0.0)
        ).classes[0].occ_a
        self.assertLess(slightly_less, 0.5)

    def test_inverse_unreachable_reports_not_limit(self):
        res = invert_dose(SITES, "receptor", 0.999, total_b=1e5, limit_a=10.0)
        self.assertFalse(res.achieved)
        self.assertIsNone(res.total_a)
        self.assertIsNone(res.equilibrium)
        self.assertLess(res.occupancy_at_limit, 0.999)
        self.assertIn("unreachable", res.message)

    def test_inverse_zero_target(self):
        res = invert_dose(SITES, "receptor", 0.0, total_b=5.0, limit_a=100.0)
        self.assertTrue(res.achieved)
        self.assertEqual(res.total_a, 0.0)
        self.assertEqual(res.occupancy_at_solution, 0.0)

    def test_inverse_validation(self):
        with self.assertRaises(ValueError):
            invert_dose(SITES, "nope", 0.5, 0.0, 10.0)
        with self.assertRaises(ValueError):
            invert_dose(SITES, "receptor", 1.0, 0.0, 10.0)
        with self.assertRaises(ValueError):
            invert_dose(SITES, "receptor", -0.1, 0.0, 10.0)


if __name__ == "__main__":
    unittest.main()
