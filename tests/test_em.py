import math
import unittest

from circular_mixture.em import Component, e_step, fit


def deg(d):
    return math.radians(d) % (2 * math.pi)


class TestWrapAroundFit(unittest.TestCase):
    def setUp(self):
        # Cluster straddling 0 deg plus a cluster near 180 deg.
        self.angles = [deg(d) for d in (358, 359, 0, 1, 2, 178, 179, 180, 181, 182)]
        self.weights = [1.0] * len(self.angles)
        self.init = [
            Component(0.5, deg(355.0), 5.0),
            Component(0.5, deg(185.0), 5.0),
        ]

    def test_means_not_flipped(self):
        res = fit(self.angles, self.weights, self.init)
        self.assertTrue(res.converged, res.message)
        mus = sorted(c.mu for c in res.components)
        # One mean near 0/2*pi, one near pi; none near the "linear average".
        near_zero = min(mus[0], 2 * math.pi - mus[0])
        self.assertLess(near_zero, math.radians(5))
        self.assertAlmostEqual(mus[1], math.pi, delta=math.radians(5))

    def test_responsibilities_sum_to_one(self):
        res = fit(self.angles, self.weights, self.init)
        for row in res.responsibilities:
            self.assertAlmostEqual(sum(row), 1.0, places=12)
            for p in row:
                self.assertGreaterEqual(p, 0.0)
                self.assertLessEqual(p, 1.0)

    def test_weights_sum_to_one(self):
        res = fit(self.angles, self.weights, self.init)
        self.assertAlmostEqual(sum(c.weight for c in res.components), 1.0, places=12)
        for c in res.components:
            self.assertGreaterEqual(c.weight, 0.0)
            self.assertLessEqual(c.kappa, 50.0)

    def test_log_likelihood_monotone(self):
        res = fit(self.angles, self.weights, self.init)
        hist = res.log_likelihood_history
        for a, b in zip(hist, hist[1:]):
            self.assertGreaterEqual(b, a - 1e-9)


class TestUniform(unittest.TestCase):
    def test_uniform_data_kappa_zero(self):
        angles = [i * math.pi / 4 for i in range(8)]
        weights = [1.0] * 8
        res = fit(angles, weights, [Component(1.0, 0.0, 1.0)])
        self.assertTrue(res.converged, res.message)
        self.assertAlmostEqual(res.components[0].kappa, 0.0, places=6)
        # Uniform log-likelihood: n * -log(2*pi)
        self.assertAlmostEqual(res.log_likelihood, -8 * math.log(2 * math.pi), places=6)


class TestWeights(unittest.TestCase):
    def test_weighted_fit_shifts_mean(self):
        angles = [deg(0.0), deg(90.0)]
        weights = [9.0, 1.0]
        res = fit(angles, weights, [Component(1.0, deg(45.0), 1.0)])
        mu = res.components[0].mu
        self.assertLess(min(mu, 2 * math.pi - mu), math.radians(30))

    def test_duplicate_weighting_equivalence(self):
        angles = [deg(10.0), deg(200.0)]
        res_a = fit(angles, [3.0, 1.0],
                    [Component(0.5, deg(0.0), 2.0), Component(0.5, deg(190.0), 2.0)])
        angles_b = [deg(10.0)] * 3 + [deg(200.0)]
        res_b = fit(angles_b, [1.0] * 4,
                    [Component(0.5, deg(0.0), 2.0), Component(0.5, deg(190.0), 2.0)])
        for ca, cb in zip(res_a.components, res_b.components):
            self.assertAlmostEqual(ca.weight, cb.weight, places=10)
            self.assertAlmostEqual(ca.mu, cb.mu, places=10)
            self.assertAlmostEqual(ca.kappa, cb.kappa, places=10)


class TestNonConvergence(unittest.TestCase):
    def test_iteration_cap_not_converged(self):
        angles = [deg(d) for d in (0, 10, 170, 180)]
        res = fit(angles, [1.0] * 4,
                  [Component(0.5, deg(5.0), 1.0), Component(0.5, deg(175.0), 1.0)],
                  tol=1e-15, max_iter=2)
        self.assertFalse(res.converged)
        self.assertEqual(res.iterations, 2)
        self.assertTrue(math.isfinite(res.log_likelihood))


class TestEmptyComponent(unittest.TestCase):
    def test_empty_component_stays_finite(self):
        # Third component starts far from all data and gets no responsibility.
        angles = [deg(d) for d in (0, 1, 359, 358)]
        init = [
            Component(0.9, deg(0.0), 10.0),
            Component(0.05, deg(120.0), 50.0),
            Component(0.05, deg(240.0), 50.0),
        ]
        res = fit(angles, [1.0] * 4, init)
        for c in res.components:
            self.assertTrue(math.isfinite(c.mu))
            self.assertTrue(math.isfinite(c.kappa))
            self.assertGreaterEqual(c.weight, 0.0)
        self.assertTrue(math.isfinite(res.log_likelihood))


if __name__ == "__main__":
    unittest.main()
