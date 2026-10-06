import unittest

from soil_storage import (
    Layer,
    invert_water_table,
    profile_storage,
    theta_at_depth,
)
from soil_storage.model import layer_index_at, van_genuchten_theta


def make_layers():
    return [
        Layer(thickness=30.0, theta_r=0.05, theta_s=0.45, alpha=0.02, n=1.5),
        Layer(thickness=50.0, theta_r=0.08, theta_s=0.50, alpha=0.05, n=2.0),
        Layer(thickness=20.0, theta_r=0.10, theta_s=0.40, alpha=0.10, n=3.0),
    ]


class TestValidation(unittest.TestCase):
    def test_invalid_layers(self):
        with self.assertRaises(ValueError):
            Layer(0.0, 0.05, 0.45, 0.02, 1.5)  # non-positive thickness
        with self.assertRaises(ValueError):
            Layer(10.0, 0.45, 0.45, 0.02, 1.5)  # theta_r >= theta_s
        with self.assertRaises(ValueError):
            Layer(10.0, 0.05, 1.2, 0.02, 1.5)  # theta_s > 1
        with self.assertRaises(ValueError):
            Layer(10.0, 0.05, 0.45, -0.02, 1.5)  # alpha <= 0
        with self.assertRaises(ValueError):
            Layer(10.0, 0.05, 0.45, 0.02, 1.0)  # n <= 1
        with self.assertRaises(ValueError):
            Layer(10.0, 0.05, 0.45, float("nan"), 1.5)  # non-finite

    def test_m_relation(self):
        layer = Layer(10.0, 0.05, 0.45, 0.02, 2.0)
        self.assertAlmostEqual(layer.m, 1.0 - 1.0 / layer.n)


class TestRetention(unittest.TestCase):
    def test_saturated_at_zero_suction(self):
        layer = make_layers()[0]
        self.assertEqual(van_genuchten_theta(layer, 0.0), layer.theta_s)
        self.assertEqual(van_genuchten_theta(layer, -5.0), layer.theta_s)

    def test_residual_limit(self):
        layer = make_layers()[0]
        theta = van_genuchten_theta(layer, 1e12)
        self.assertAlmostEqual(theta, layer.theta_r, places=4)


class TestStorage(unittest.TestCase):
    def test_fully_saturated(self):
        layers = make_layers()
        result = profile_storage(layers, 0.0)
        expected = sum(l.theta_s * l.thickness for l in layers)
        self.assertAlmostEqual(result.total, expected, places=9)
        self.assertTrue(result.converged)
        self.assertGreater(result.tolerance, 0.0)

    def test_layer_boundary_split(self):
        # Water table exactly on the boundary between layer 1 and 2.
        layers = make_layers()
        result = profile_storage(layers, 30.0)
        # Layers 2 and 3 fully saturated.
        self.assertAlmostEqual(result.layers[1].storage, 25.0, places=9)
        self.assertAlmostEqual(result.layers[2].storage, 8.0, places=9)
        # Layer 1 storage must equal integral of theta over h in [0, 30].
        layer = layers[0]
        steps = 200000
        h = 30.0 / steps
        acc = 0.0
        for i in range(steps + 1):
            w = 0.5 if i in (0, steps) else 1.0
            x = i * h
            acc += w * (layer.theta_r + (layer.theta_s - layer.theta_r)
                        * (1.0 + (layer.alpha * x) ** layer.n) ** (-layer.m))
        self.assertAlmostEqual(result.layers[0].storage, acc * h, places=6)

    def test_storage_decreases_with_depth(self):
        layers = make_layers()
        totals = [profile_storage(layers, d).total
                  for d in (0.0, 10.0, 30.0, 55.0, 100.0, 300.0)]
        for a, b in zip(totals, totals[1:]):
            self.assertGreater(a, b)

    def test_not_midpoint_approximation(self):
        # Integral must differ from theta(midpoint) * thickness in general.
        layers = make_layers()
        result = profile_storage(layers, 60.0)
        layer = layers[0]
        midpoint_theta = van_genuchten_theta(layer, 60.0 - 15.0)
        self.assertNotAlmostEqual(result.layers[0].storage,
                                  midpoint_theta * 30.0, places=4)


class TestSampling(unittest.TestCase):
    def test_boundary_takes_lower_layer(self):
        layers = make_layers()
        self.assertEqual(layer_index_at(layers, 0.0), 0)
        self.assertEqual(layer_index_at(layers, 30.0), 1)  # boundary -> lower
        self.assertEqual(layer_index_at(layers, 80.0), 2)
        self.assertEqual(layer_index_at(layers, 100.0), 2)  # bottom -> last

    def test_out_of_range_rejected(self):
        layers = make_layers()
        with self.assertRaises(ValueError):
            layer_index_at(layers, -0.1)
        with self.assertRaises(ValueError):
            layer_index_at(layers, 100.1)
        with self.assertRaises(ValueError):
            theta_at_depth(layers, 10.0, 101.0)

    def test_theta_at_depth_below_water_table(self):
        layers = make_layers()
        self.assertEqual(theta_at_depth(layers, 10.0, 50.0),
                         layers[1].theta_s)


class TestInversion(unittest.TestCase):
    def test_roundtrip(self):
        layers = make_layers()
        target = profile_storage(layers, 45.0).total
        res = invert_water_table(layers, target, 200.0)
        self.assertTrue(res.solved)
        self.assertAlmostEqual(res.water_table, 45.0, places=6)
        self.assertAlmostEqual(res.residual, 0.0, places=7)
        # Back-substitution check.
        check = profile_storage(layers, res.water_table).total
        self.assertAlmostEqual(check, target, places=7)

    def test_shallowest_solution(self):
        layers = make_layers()
        target = profile_storage(layers, 0.0).total
        res = invert_water_table(layers, target, 200.0)
        self.assertTrue(res.solved)
        self.assertAlmostEqual(res.water_table, 0.0, places=8)

    def test_out_of_range_no_fake_boundary(self):
        layers = make_layers()
        s_max = profile_storage(layers, 0.0).total
        res = invert_water_table(layers, s_max + 1.0, 200.0)
        self.assertFalse(res.solved)
        self.assertIsNone(res.water_table)
        self.assertIsNone(res.residual)
        s_min = profile_storage(layers, 200.0).total
        res2 = invert_water_table(layers, s_min - 1.0, 200.0)
        self.assertFalse(res2.solved)
        self.assertIsNone(res2.water_table)

    def test_invalid_inputs(self):
        layers = make_layers()
        with self.assertRaises(ValueError):
            invert_water_table(layers, -1.0, 200.0)
        with self.assertRaises(ValueError):
            invert_water_table(layers, 10.0, 0.0)


if __name__ == "__main__":
    unittest.main()
