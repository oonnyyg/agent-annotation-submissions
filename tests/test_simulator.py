"""Core simulator tests."""
import math
import unittest

from dsn.config import load_config
from dsn.simulator import simulate


def base_config(**overrides):
    cfg = {
        "dt": 1.0,
        "steps": 10,
        "neurons": [
            {
                "name": "a",
                "tau": 5.0,
                "threshold": 1.0,
                "reset": 0.0,
                "v0": 0.0,
                "refractory_steps": 0,
                "stimulus": [{"start": 0.0, "end": 10.0, "v_eq": 0.0}],
            }
        ],
        "connections": [],
    }
    cfg.update(overrides)
    return cfg


class SingleNeuronTests(unittest.TestCase):
    def test_closed_form_evolution(self):
        cfg = base_config()
        cfg["neurons"][0]["stimulus"] = [{"start": 0.0, "end": 10.0, "v_eq": 0.5}]
        result = simulate(load_config(cfg))
        decay = math.exp(-1.0 / 5.0)
        expected = 0.5 * (1 - decay)
        self.assertAlmostEqual(result.neurons[0].voltages[0], expected)
        self.assertAlmostEqual(
            result.neurons[0].voltages[1], 0.5 + (expected - 0.5) * decay
        )

    def test_spike_and_reset(self):
        cfg = base_config()
        cfg["neurons"][0]["stimulus"] = [{"start": 0.0, "end": 10.0, "v_eq": 10.0}]
        result = simulate(load_config(cfg))
        n = result.neurons[0]
        self.assertEqual(n.spike_steps[0], 0)
        self.assertEqual(n.voltages[0], 0.0)
        self.assertEqual(n.spike_count, len(n.spike_steps))
        self.assertAlmostEqual(n.mean_rate(1.0), n.spike_count / 10.0)

    def test_refractory_holds_reset_and_skips_steps(self):
        cfg = base_config()
        neuron = cfg["neurons"][0]
        neuron["refractory_steps"] = 2
        neuron["stimulus"] = [{"start": 0.0, "end": 10.0, "v_eq": 10.0}]
        result = simulate(load_config(cfg))
        n = result.neurons[0]
        self.assertEqual(n.spike_steps, [0, 3, 6, 9])
        self.assertEqual(n.voltages[1], 0.0)
        self.assertEqual(n.voltages[2], 0.0)

    def test_zero_refractory_no_extra_skips(self):
        cfg = base_config()
        cfg["neurons"][0]["stimulus"] = [{"start": 0.0, "end": 10.0, "v_eq": 10.0}]
        result = simulate(load_config(cfg))
        self.assertEqual(result.neurons[0].spike_steps, list(range(10)))


if __name__ == "__main__":
    unittest.main()
