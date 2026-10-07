"""Network interaction tests: delays, inhibition, simultaneous arrivals."""
import unittest
from dsn.config import load_config
from dsn.simulator import simulate


def two_neuron_config(connections, v_eq_a=10.0, v_eq_b=0.0, steps=10):
    return {
        "dt": 1.0,
        "steps": steps,
        "neurons": [
            {
                "name": "a",
                "tau": 5.0,
                "threshold": 1.0,
                "reset": 0.0,
                "v0": 0.0,
                "refractory_steps": 0,
                "stimulus": [{"start": 0.0, "end": float(steps), "v_eq": v_eq_a}],
            },
            {
                "name": "b",
                "tau": 5.0,
                "threshold": 1.0,
                "reset": 0.0,
                "v0": 0.0,
                "refractory_steps": 0,
                "stimulus": [{"start": 0.0, "end": float(steps), "v_eq": v_eq_b}],
            }
        ],
        "connections": connections,
    }


class NetworkTests(unittest.TestCase):
    def test_excitatory_delivery_after_delay(self):
        cfg = two_neuron_config(
            [{"from": "a", "to": "b", "weight": 0.4, "delay": 3}], v_eq_b=0.0
        )
        result = simulate(load_config(cfg))
        b = result.neurons[1]
        # a spikes at t=0; b receives +0.4 at end of t=3.
        self.assertAlmostEqual(b.voltages[2], 0.0)
        self.assertAlmostEqual(b.voltages[3], 0.4)

    def test_inhibitory_connection_prevents_spike(self):
        # b would spike on its own (v_eq=2.0) but a inhibits it at t=1.
        cfg = two_neuron_config(
            [{"from": "a", "to": "b", "weight": -2.0, "delay": 1}], v_eq_b=2.0
        )
        result = simulate(load_config(cfg))
        b = result.neurons[1]
        # a spikes at t=0, -2.0 lands on b at end of t=1, cancelling its rise.
        self.assertEqual(b.spike_steps, [])
        self.assertLess(b.voltages[1], 0.0)

    def test_simultaneous_arrivals_sum(self):
        cfg = two_neuron_config(
            [
                {"from": "a", "to": "b", "weight": 0.6, "delay": 1},
                {"from": "a", "to": "b", "weight": 0.6, "delay": 1},
            ],
            v_eq_b=0.0,
        )
        result = simulate(load_config(cfg))
        b = result.neurons[1]
        # Both jumps land at end of t=1: 1.2 >= threshold, b spikes at t=1.
        self.assertEqual(b.spike_steps[0], 1)
        self.assertEqual(b.voltages[1], 0.0)  # post-reset

    def test_same_step_spikes_do_not_cascade(self):
        # a->b delay 1: b's spike at t=1 is from a's t=0 spike, not t=1's.
        cfg = two_neuron_config(
            [{"from": "a", "to": "b", "weight": 1.5, "delay": 1}], v_eq_b=0.0
        )
        result = simulate(load_config(cfg))
        a, b = result.neurons
        self.assertEqual(a.spike_steps, list(range(10)))
        self.assertEqual(b.spike_steps, list(range(1, 10)))

    def test_delayed_self_connection(self):
        cfg = two_neuron_config(
            [{"from": "a", "to": "a", "weight": -2.0, "delay": 2}], v_eq_b=0.0
        )
        result = simulate(load_config(cfg))
        a = result.neurons[0]
        # Self-inhibition from spikes at t=0,1 lands at t=2,3, silencing them.
        self.assertNotIn(2, a.spike_steps)
        self.assertNotIn(3, a.spike_steps)
        self.assertIn(4, a.spike_steps)
        self.assertIn(5, a.spike_steps)

    def test_undelivered_spikes_counted_not_delivered(self):
        cfg = two_neuron_config(
            [{"from": "a", "to": "b", "weight": 0.4, "delay": 5}],
            v_eq_b=0.0,
            steps=4,
        )
        result = simulate(load_config(cfg))
        b = result.neurons[1]
        self.assertEqual(b.spike_steps, [])
        self.assertEqual(b.undelivered, 4)  # a spikes at t=0..3, all arrive after end

    def test_neuron_order_does_not_change_results(self):
        cfg = two_neuron_config(
            [
                {"from": "a", "to": "b", "weight": 1.5, "delay": 1},
                {"from": "b", "to": "a", "weight": -0.3, "delay": 2},
            ],
            v_eq_b=2.0,
        )
        first = simulate(load_config(cfg))
        cfg["neurons"] = list(reversed(cfg["neurons"]))
        second = simulate(load_config(cfg))
        by_name = {n.name: n for n in second.neurons}
        for n in first.neurons:
            self.assertEqual(n.spike_steps, by_name[n.name].spike_steps)
            self.assertEqual(n.voltages, by_name[n.name].voltages)


if __name__ == "__main__":
    unittest.main()
