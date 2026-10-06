import unittest

from flash_balance.flash import FlashError, solve_flash


class FlashTests(unittest.TestCase):
    def test_two_phase_split(self):
        result = solve_flash({"a": 50.0, "b": 50.0}, {"a": 2.0, "b": 0.5})
        self.assertEqual(result.phase, "two_phase")
        self.assertAlmostEqual(result.vapor_fraction, 0.5, places=9)
        for comp in ("a", "b"):
            self.assertAlmostEqual(
                result.liquid_flows[comp] + result.vapor_flows[comp], 50.0, places=9)
        # Equilibrium check: y_i = K_i * x_i.
        for comp, ki in (("a", 2.0), ("b", 0.5)):
            x = result.liquid_flows[comp] / sum(result.liquid_flows.values())
            y = result.vapor_flows[comp] / sum(result.vapor_flows.values())
            self.assertAlmostEqual(y, ki * x, places=9)

    def test_all_liquid_endpoint(self):
        result = solve_flash({"a": 10.0}, {"a": 0.1})
        self.assertEqual(result.phase, "liquid")
        self.assertEqual(result.vapor_fraction, 0.0)
        self.assertEqual(result.vapor_flows["a"], 0.0)

    def test_all_vapor_endpoint(self):
        result = solve_flash({"a": 10.0}, {"a": 10.0})
        self.assertEqual(result.phase, "vapor")
        self.assertEqual(result.vapor_fraction, 1.0)
        self.assertEqual(result.liquid_flows["a"], 0.0)

    def test_degenerate_all_k_one(self):
        result = solve_flash({"a": 3.0, "b": 7.0}, {"a": 1.0, "b": 1.0})
        self.assertTrue(result.degenerate)
        self.assertEqual(result.phase, "liquid")
        self.assertEqual(result.liquid_flows, {"a": 3.0, "b": 7.0})

    def test_near_one_k_converges(self):
        result = solve_flash({"a": 50.05, "b": 49.95}, {"a": 1.001, "b": 0.999})
        self.assertEqual(result.phase, "two_phase")
        self.assertLess(abs(result.residual), 1e-9)

    def test_zero_total_rejected(self):
        with self.assertRaises(FlashError):
            solve_flash({"a": 0.0}, {"a": 2.0})

    def test_negative_flow_rejected(self):
        with self.assertRaises(FlashError):
            solve_flash({"a": -1.0}, {"a": 2.0})

    def test_bad_k_rejected(self):
        with self.assertRaises(FlashError):
            solve_flash({"a": 1.0}, {"a": 0.0})
        with self.assertRaises(FlashError):
            solve_flash({"a": 1.0}, {})


if __name__ == "__main__":
    unittest.main()
