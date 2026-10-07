"""Tests for the delayed LIF network simulator (standard library only)."""

from __future__ import annotations

import math
import unittest

from lif_config import ConfigError, build_network
from lif_sim import Network, NeuronConfig, SynapseConfig


def make_network(
    neurons: list[NeuronConfig],
    synapses: list[SynapseConfig] | None = None,
    equilibrium_value: float | list[float] = 0.0,
    num_steps: int = 10,
    dt: float = 1.0,
) -> Network:
    names = [f"n{i}" for i in range(len(neurons))]
    if isinstance(equilibrium_value, list):
        row = list(equilibrium_value)
    else:
        row = [equilibrium_value] * len(neurons)
    equilibrium = [list(row) for _ in range(num_steps)]
    return Network(
        dt=dt,
        num_steps=num_steps,
        neurons=neurons,
        neuron_names=names,
        synapses=synapses or [],
        equilibrium=equilibrium,
    )


class SingleNeuronTests(unittest.TestCase):
    def test_closed_form_relaxation(self):
        neuron = NeuronConfig(tau=2.0, threshold=10.0, reset=-1.0, initial=-1.0)
        net = make_network([neuron], equilibrium_value=3.0, num_steps=3, dt=0.5)
        result = net.run()
        decay = math.exp(-0.5 / 2.0)
        v = -1.0
        for step in range(3):
            v = 3.0 + (v - 3.0) * decay
            self.assertAlmostEqual(result.voltages[step][0], v, places=12)

    def test_spike_reset_and_one_spike_per_step(self):
        # Huge equilibrium: neuron would cross threshold many times in
        # one step, but it must spike at most once and reset.
        neuron = NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0)
        net = make_network([neuron], equilibrium_value=1000.0, num_steps=3)
        result = net.run()
        self.assertEqual(result.spike_counts, [3])
        for row in result.voltages:
            self.assertEqual(row[0], 0.0)
        self.assertEqual(result.spike_times[0], [1.0, 2.0, 3.0])
        # 3 spikes over 3 ms -> 1000 Hz.
        self.assertAlmostEqual(result.mean_rates_hz[0], 1000.0)

    def test_subthreshold_never_spikes(self):
        neuron = NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0)
        net = make_network([neuron], equilibrium_value=0.5, num_steps=50)
        result = net.run()
        self.assertEqual(result.spike_counts, [0])


class SynapseTests(unittest.TestCase):
    def test_delayed_delivery_not_same_step(self):
        # n0 spikes on step 0; with delay 2 the jump must arrive at the
        # end of step 2, not earlier.
        neurons = [
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
        ]
        syn = SynapseConfig(source=0, target=1, weight=5.0, delay_steps=2)
        # n1 sits at a subthreshold equilibrium so it can only be
        # driven across threshold by the delayed jump.
        net = make_network(
            neurons, [syn], equilibrium_value=[2.0, 0.0], num_steps=4
        )
        result = net.run()
        # n0 spikes every step; each jump arrives 2 steps later, so n1
        # first spikes at the end of step 2 (t = 3 ms), never earlier.
        self.assertEqual(result.spike_times[1], [3.0, 4.0])
        self.assertEqual(result.voltages[0][1], 0.0)
        self.assertEqual(result.voltages[2][1], 0.0)  # reset after spike

    def test_inhibitory_synapse_prevents_spike(self):
        # n0 spikes every step (eq 2 > thr 1). n1 has a suprathreshold
        # equilibrium but a large tau, so it approaches threshold
        # slowly; the negative jumps must keep it below threshold.
        neurons = [
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
            NeuronConfig(tau=10.0, threshold=1.0, reset=0.0, initial=0.0),
        ]
        syn = SynapseConfig(source=0, target=1, weight=-100.0, delay_steps=1)
        net = make_network(neurons, [syn], equilibrium_value=2.0, num_steps=5)
        result = net.run()
        self.assertEqual(result.spike_counts[0], 5)
        self.assertEqual(result.spike_counts[1], 0)
        for row in result.voltages:
            self.assertLess(row[1], 1.0)

    def test_same_step_inputs_sum_and_are_order_independent(self):
        # Two presynaptic neurons spike on the same step; both jumps
        # must arrive together at the target regardless of synapse
        # listing order.
        neurons = [
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
        ]
        synapses = [
            SynapseConfig(source=0, target=2, weight=0.6, delay_steps=1),
            SynapseConfig(source=1, target=2, weight=0.6, delay_steps=1),
        ]
        # n2 sits at rest; only the summed jumps (0.6 + 0.6 >= 1.0)
        # can push it over threshold.
        kwargs = dict(equilibrium_value=[2.0, 2.0, 0.0], num_steps=3)
        forward = make_network(neurons, synapses, **kwargs).run()
        reversed_synapses = list(reversed(synapses))
        backward = make_network(neurons, reversed_synapses, **kwargs).run()
        # n2 spikes at the end of steps 1 and 2 in both orderings.
        self.assertEqual(forward.spike_times[2], [2.0, 3.0])
        self.assertEqual(forward.spike_times, backward.spike_times)
        self.assertEqual(forward.voltages, backward.voltages)

    def test_delayed_self_connection(self):
        # Self-connection with delay 1: spike at step t adds +0.5 at
        # the end of step t+1.
        neuron = NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0)
        syn = SynapseConfig(source=0, target=0, weight=0.5, delay_steps=1)
        net = make_network([neuron], [syn], equilibrium_value=2.0, num_steps=4)
        result = net.run()
        # Step 0: relax to 2(1-e^-1) ~ 1.264 >= 1 -> spike, reset.
        # Step 1: relax to 1.264, plus self jump 0.5 -> spike again.
        self.assertEqual(result.spike_times[0], [1.0, 2.0, 3.0, 4.0])

    def test_undelivered_spikes_counted(self):
        neurons = [
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
        ]
        syn = SynapseConfig(source=0, target=1, weight=1.0, delay_steps=3)
        net = make_network(
            neurons, [syn], equilibrium_value=[2.0, 0.0], num_steps=2
        )
        result = net.run()
        # n0 spikes at steps 0 and 1; arrivals at steps 3 and 4 are
        # beyond the horizon and must only be counted as undelivered.
        self.assertEqual(result.undelivered_spikes, 2)
        self.assertEqual(result.spike_counts[1], 0)


class RefractoryTests(unittest.TestCase):
    def test_refractory_skips_full_steps_and_discards_input(self):
        neurons = [
            NeuronConfig(tau=1.0, threshold=1.0, reset=0.0, initial=0.0),
            NeuronConfig(
                tau=1.0, threshold=1.0, reset=0.0, initial=0.0,
                refractory_steps=2,
            ),
        ]
        # n0 spikes every step and sends +50 to n1 every step; n1 must
        # discard arrivals during its 2 refractory steps.
        syn = SynapseConfig(source=0, target=1, weight=50.0, delay_steps=1)
        net = make_network(neurons, [syn], equilibrium_value=2.0, num_steps=8)
        result = net.run()
        # n1 spikes at end of step 0, held at reset on steps 1 and 2
        # (inputs discarded), evolves again on step 3 and spikes, etc.
        self.assertEqual(result.spike_times[1], [1.0, 4.0, 7.0])
        self.assertEqual(result.voltages[1][1], 0.0)
        self.assertEqual(result.voltages[2][1], 0.0)

    def test_zero_refractory_means_no_skipped_steps(self):
        neuron = NeuronConfig(
            tau=1.0, threshold=1.0, reset=0.0, initial=0.0, refractory_steps=0
        )
        net = make_network([neuron], equilibrium_value=2.0, num_steps=4)
        result = net.run()
        self.assertEqual(result.spike_counts, [4])


class ConfigTests(unittest.TestCase):
    def base_config(self) -> dict:
        return {
            "dt": 0.5,
            "duration": 10.0,
            "neurons": [
                {
                    "name": "A",
                    "tau": 10.0,
                    "threshold": -50.0,
                    "reset": -65.0,
                    "initial": -70.0,
                }
            ],
            "synapses": [],
            "stimulus": {"A": [[0.0, 10.0, -60.0]]},
        }

    def test_valid_config_builds(self):
        net = build_network(self.base_config())
        self.assertEqual(net.num_steps, 20)
        self.assertEqual(net.equilibrium[0][0], -60.0)
        self.assertEqual(net.equilibrium[19][0], -60.0)

    def test_reset_must_be_below_threshold(self):
        cfg = self.base_config()
        cfg["neurons"][0]["reset"] = -40.0
        with self.assertRaises(ConfigError):
            build_network(cfg)

    def test_initial_must_be_below_threshold(self):
        cfg = self.base_config()
        cfg["neurons"][0]["initial"] = -45.0
        with self.assertRaises(ConfigError):
            build_network(cfg)

    def test_stimulus_must_align_to_grid(self):
        cfg = self.base_config()
        cfg["stimulus"] = {"A": [[0.0, 5.0, -60.0], [5.3, 10.0, -60.0]]}
        with self.assertRaises(ConfigError):
            build_network(cfg)

    def test_stimulus_must_cover_full_duration(self):
        cfg = self.base_config()
        cfg["stimulus"] = {"A": [[0.0, 8.0, -60.0]]}
        with self.assertRaises(ConfigError):
            build_network(cfg)

    def test_stimulus_overlap_rejected(self):
        cfg = self.base_config()
        cfg["stimulus"] = {
            "A": [[0.0, 6.0, -60.0], [5.0, 10.0, -60.0]]
        }
        with self.assertRaises(ConfigError):
            build_network(cfg)

    def test_delay_must_be_at_least_one(self):
        cfg = self.base_config()
        cfg["synapses"] = [
            {"source": "A", "target": "A", "weight": 1.0, "delay_steps": 0}
        ]
        with self.assertRaises(ConfigError):
            build_network(cfg)

    def test_unknown_synapse_endpoint_rejected(self):
        cfg = self.base_config()
        cfg["synapses"] = [
            {"source": "A", "target": "B", "weight": 1.0, "delay_steps": 1}
        ]
        with self.assertRaises(ConfigError):
            build_network(cfg)

    def test_piecewise_stimulus_applied_per_step(self):
        cfg = self.base_config()
        cfg["stimulus"] = {"A": [[0.0, 5.0, -60.0], [5.0, 10.0, -55.0]]}
        net = build_network(cfg)
        for step in range(10):
            self.assertEqual(net.equilibrium[step][0], -60.0)
        for step in range(10, 20):
            self.assertEqual(net.equilibrium[step][0], -55.0)


if __name__ == "__main__":
    unittest.main()
