"""Fixed-time-step leaky integrate-and-fire network playback.

Per step t (in order):
  1. Non-refractory neurons evolve in closed form toward this step's
     equilibrium voltage: v += (v_eq - v) * (1 - exp(-dt / tau)).
  2. All synaptic jumps scheduled to arrive at the end of step t are added.
  3. Every neuron with v >= threshold spikes (at most once per step) and is
     reset immediately; spikes are scheduled for delivery at t + delay.

Refractory neurons hold their reset voltage for the configured number of
full subsequent steps, discard arriving pulses, and skip the evolution.
"""

from __future__ import annotations

import math
from collections import defaultdict
from dataclasses import dataclass, field

from .config import Config


@dataclass
class NeuronResult:
    name: str
    voltages: list[float]  # post-reset voltage at the end of each step
    spike_steps: list[int]
    undelivered: int  # spikes still in transit when the run ended

    @property
    def spike_count(self) -> int:
        return len(self.spike_steps)

    def mean_rate(self, dt: float) -> float:
        return self.spike_count / (dt * len(self.voltages))


@dataclass
class SimulationResult:
    dt: float
    steps: int
    neurons: list[NeuronResult] = field(default_factory=list)

    def summary(self) -> list[dict]:
        return [
            {
                "name": n.name,
                "spike_count": n.spike_count,
                "mean_rate": n.mean_rate(self.dt),
                "spike_steps": list(n.spike_steps),
                "undelivered": n.undelivered,
            }
            for n in self.neurons
        ]


def simulate(config: Config) -> SimulationResult:
    neurons = config.neurons
    index_of = {n.name: i for i, n in enumerate(neurons)}

    # Precompute per-step equilibrium voltage for each neuron.
    v_eq_table = []
    for n in neurons:
        row = [0.0] * config.steps
        for seg in n.stimulus:
            for t in range(seg.start_step, seg.end_step):
                row[t] = seg.v_eq
        v_eq_table.append(row)

    decay = [math.exp(-config.dt / n.tau) for n in neurons]
    voltage = [n.v0 for n in neurons]
    refractory_left = [0] * len(neurons)

    # arrivals[t] -> list of (target_index, weight); integer steps, no float rounding.
    arrivals: dict[int, list[tuple[int, float]]] = defaultdict(list)
    outgoing = [[] for _ in neurons]  # source index -> [(target index, weight, delay)]
    for c in config.connections:
        outgoing[index_of[c.source]].append((index_of[c.target], c.weight, c.delay_steps))

    results = [NeuronResult(n.name, [], [], 0) for n in neurons]

    for t in range(config.steps):
        # 1. Closed-form exponential evolution toward this step's equilibrium.
        for i, n in enumerate(neurons):
            if refractory_left[i] > 0:
                continue
            v_eq = v_eq_table[i][t]
            voltage[i] = v_eq + (voltage[i] - v_eq) * decay[i]

        # 2. Deliver all jumps arriving at the end of this step.
        for target, weight in arrivals.pop(t, []):
            if refractory_left[target] > 0:
                continue  # discarded while refractory
            voltage[target] += weight

        # 3. Threshold check; spikes reset immediately and schedule deliveries.
        for i, n in enumerate(neurons):
            if refractory_left[i] > 0:
                refractory_left[i] -= 1
                voltage[i] = n.reset
            elif voltage[i] >= n.threshold:
                results[i].spike_steps.append(t)
                voltage[i] = n.reset
                refractory_left[i] = n.refractory_steps
                for target, weight, delay in outgoing[i]:
                    arrivals[t + delay].append((target, weight))
            results[i].voltages.append(voltage[i])

    # Spikes still in transit past the final step are reported, not delivered.
    for t, pending in arrivals.items():
        if t >= config.steps:
            for target, _ in pending:
                results[target].undelivered += 1

    return SimulationResult(config.dt, config.steps, results)
