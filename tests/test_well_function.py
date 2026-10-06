
import math
import unittest

from aquifer_response.well_function import WellFunctionError, theis_w


class TheisWellFunctionTest(unittest.TestCase):
    def test_known_values(self):
        cases = [
            (1e-6, 13.238295893062741),
            (0.001, 6.33153936413615),
            (0.5, 0.5597735947761608),
            (1.0, 0.21938393439552029),
            (5.0, 0.0011482955912753257),
            (20.0, 9.835525290649884e-11),
            (50.0, 3.783264029550459e-24),
        ]
        for u, expected in cases:
            with self.subTest(u=u):
                self.assertAlmostEqual(theis_w(u), expected, delta=abs(expected) * 1e-12)

    def test_very_large_u_underflows_to_zero(self):
        self.assertEqual(theis_w(1e10), 0.0)

    def test_small_u_matches_log_asymptote(self):
        u = 1e-8
        expected = -0.5772156649015328606 - math.log(u) + u
        self.assertAlmostEqual(theis_w(u), expected, places=8)

    def test_invalid_u_raises_with_reason(self):
        for bad in (0.0, -1.0, float("inf"), float("nan")):
            with self.subTest(u=bad):
                with self.assertRaises(WellFunctionError) as ctx:
                    theis_w(bad)
                self.assertTrue(str(ctx.exception))

    def test_convergence_failure_reports_reason(self):
        with self.assertRaisesRegex(WellFunctionError, "did not converge"):
            theis_w(0.9, tol=1e-16, max_terms=2)

    def test_repeat_calls_are_deterministic(self):
        self.assertEqual(theis_w(3.7), theis_w(3.7))


if __name__ == "__main__":
    unittest.main()
