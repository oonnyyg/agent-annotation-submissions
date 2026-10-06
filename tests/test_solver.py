import math
import unittest

from catenary_clearance.solver import CatenarySolution, SolveError


class SolverTest(unittest.TestCase):
    def test_equal_height_known_solution(self):
        sol = CatenarySolution(0.0, 10.0, 100.0, 10.0, 120.0, 1.0)
        self.assertAlmostEqual(sol.xv, 50.0, places=9)
        self.assertAlmostEqual(sol.y(0.0), 10.0, places=9)
        self.assertAlmostEqual(sol.y(100.0), 10.0, places=9)
        self.assertAlmostEqual(sol.arc_length(0.0, 100.0), 120.0, places=9)
        sag = 10.0 - sol.y(50.0)
        self.assertAlmostEqual(sag, sol.a * (math.cosh(50.0 / sol.a) - 1.0), places=9)

    def test_unequal_height_supports(self):
        sol = CatenarySolution(0.0, 0.0, 80.0, 30.0, 120.0, 2.0)
        res = sol.residuals()
        self.assertLess(res["endpoint_y"], 1e-8)
        self.assertLess(res["arc_length"], 1e-8)
        # Tension difference equals weight of the vertical drop: T1 - T0 = w*dy.
        self.assertAlmostEqual(sol.tension(80.0) - sol.tension(0.0), 2.0 * 30.0, places=6)

    def test_vertex_outside_span_reported_not_clamped(self):
        # Near-taut cable on a steep chord: vertex lies far left of x0.
        sol = CatenarySolution(0.0, 0.0, 50.0, 60.0, 79.0, 1.0)
        self.assertLess(sol.xv, 0.0)
        lx, ly = sol.lowest_point_in_span()
        self.assertEqual(lx, 0.0)
        self.assertAlmostEqual(ly, 0.0, places=9)

    def test_near_taut_converges(self):
        chord = math.hypot(100.0, 5.0)
        sol = CatenarySolution(0.0, 0.0, 100.0, 5.0, chord * 1.000001, 1.0)
        res = sol.residuals()
        self.assertLess(res["endpoint_y"], 1e-6)
        self.assertLess(res["arc_length"], 1e-6)

    def test_deep_sag_converges(self):
        sol = CatenarySolution(0.0, 0.0, 10.0, 0.0, 5000.0, 1.0)
        res = sol.residuals()
        self.assertLess(res["endpoint_y"], 1e-6)
        self.assertLess(res["arc_length"], 1e-6)

    def test_length_not_greater_than_chord_raises(self):
        with self.assertRaises(SolveError):
            CatenarySolution(0.0, 0.0, 3.0, 4.0, 5.0, 1.0)

    def test_tension_at_vertex_equals_horizontal(self):
        sol = CatenarySolution(0.0, 3.0, 40.0, 8.0, 60.0, 1.5)
        self.assertAlmostEqual(sol.tension(sol.xv), sol.horizontal_tension, places=9)


if __name__ == "__main__":
    unittest.main()
