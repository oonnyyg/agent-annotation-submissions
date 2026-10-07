import math
import unittest

from circular_mixture_fit.em import Component, final_responsibilities, fit


def deg(d):
    return math.radians(d) % (2.0 * math.pi)


class EmTests(unittest.TestCase):
    def test_wraparound_cluster_recovered(self):
        angles = [deg(d) for d in (358.0, 359.0, 0.5, 1.0, 2.0, 359.5)]
        weights = [1.0] * len(angles)
        init = [Component(weight=1.0, mu=deg(10.0), kappa=5.0)]
        result = fit(angles, weights, init, tol=1e-10, max_iter=500)
        self.assertTrue(result.converged)
        mu_deg = math.degrees(result.components[0].mu)
        self.assertTrue(mu_deg > 350.0 or mu_deg < 10.0)
        self.assertGreater(result.components[0].kappa, 1.0)

    def test_two_components_sorted_and_weights_sum_to_one(self):
        angles = [deg(d) for d in (10, 12, 8, 11, 200, 202, 198, 201)]
        weights = [1.0] * 8
        init = [
            Component(weight=0.5, mu=deg(0.0), kappa=4.0),
            Component(weight=0.5, mu=deg(180.0), kappa=4.0),
        ]
        result = fit(angles, weights, init, tol=1e-10, max_iter=500)
        self.assertTrue(result.converged)
        total = sum(c.weight for c in result.components)
        self.assertAlmostEqual(total, 1.0, places=12)
        for c in result.components:
            self.assertGreaterEqual(c.weight, 0.0)
            self.assertLessEqual(c.kappa, 50.0)

    def test_kappa_zero_is_uniform(self):
        angles = [deg(d) for d in range(0, 360, 30)]
        weights = [1.0] * len(angles)
        init = [Component(weight=1.0, mu=deg(0.0), kappa=0.0)]
        result = fit(angles, weights, init, tol=1e-12, max_iter=50)
        self.assertLess(result.components[0].kappa, 1e-6)
        expected_ll = len(angles) * (-math.log(2.0 * math.pi))
        self.assertAlmostEqual(result.log_likelihood, expected_ll, places=8)

    def test_responsibilities_sum_to_one(self):
        angles = [deg(d) for d in (5, 355, 100, 260)]
        weights = [2.0, 1.0, 1.0, 3.0]
        comps = [
            Component(weight=0.5, mu=deg(0.0), kappa=3.0),
            Component(weight=0.5, mu=deg(180.0), kappa=3.0),
        ]
        resp, ll = final_responsibilities(angles, weights, comps)
        self.assertTrue(math.isfinite(ll))
        for row in resp:
            self.assertAlmostEqual(sum(row), 1.0, places=12)

    def test_max_iter_exhaustion_is_not_converged(self):
        angles = [deg(d) for d in (0.0, 120.0, 240.0)]
        weights = [1.0] * 3
        init = [
            Component(weight=0.5, mu=deg(0.0), kappa=2.0),
            Component(weight=0.5, mu=deg(180.0), kappa=2.0),
        ]
        result = fit(angles, weights, init, tol=1e-15, max_iter=2)
        self.assertFalse(result.converged)
        self.assertIn("not converged", result.message)

    def test_empty_component_kept_at_zero_weight(self):
        angles = [deg(d) for d in (10, 12, 11, 9)]
        weights = [1.0] * 4
        init = [
            Component(weight=0.9, mu=deg(10.0), kappa=10.0),
            Component(weight=0.1, mu=deg(180.0), kappa=0.0),
        ]
        result = fit(angles, weights, init, tol=1e-12, max_iter=500)
        weights_out = sorted(c.weight for c in result.components)
        self.assertAlmostEqual(sum(weights_out), 1.0, places=12)
        self.assertTrue(all(math.isfinite(c.mu) for c in result.components))

    def test_log_likelihood_history_recorded(self):
        angles = [deg(d) for d in (0.0, 5.0, 355.0)]
        weights = [1.0] * 3
        init = [Component(weight=1.0, mu=deg(0.0), kappa=2.0)]
        result = fit(angles, weights, init)
        self.assertEqual(len(result.log_likelihood_history), result.iterations + 1)
        for a, b in zip(
            result.log_likelihood_history, result.log_likelihood_history[1:]
        ):
            self.assertGreaterEqual(b, a - 1e-9)


if __name__ == "__main__":
    unittest.main()
