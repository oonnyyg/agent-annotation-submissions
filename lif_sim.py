"""Delayed leaky integrate-and-fire (LIF) spiking network simulator.

Fixed time-step replay of external stimuli and delayed synaptic
interactions. No training is performed.

Units and conventions
---------------------
- Time is in milliseconds (ms); the time step ``dt`` is in ms.
- Voltages are in millivolts (mV).
- Time constants ``tau`` are in ms.
- Firing rates are reported in Hz (spikes per second).

Update order per step ``t`` (voltage evolves from time ``t`` to ``t+dt``):

1. Every neuron not in its refractory period is evolved with the
   closed-form exponential relaxation toward the equilibrium voltage
   ``V_eq(t)`` valid for this step::

       V <- V_eq + (V - V_eq) * exp(-dt / tau)

   Neurons in their refractory period are held exactly at the reset
   voltage and any synaptic input arriving during those steps is
   discarded.
2. All synaptic jumps scheduled to arrive at the end of this step are
   added at once (positive or negative weights).
3. Threshold crossing is decided simultaneously for all neurons:
   a neuron spikes at most once per step if ``V >= threshold`` and is
   reset immediately.
4. Spikes produced in this step are only scheduled into future steps
   (delay >= 1 step), so they can never affect the current step's
   decisions, regardless of iteration order.

Refractory handling: after a spike at step ``t``, the neuron skips
``refractory_steps`` *subsequent full steps* (held at reset, inputs
discarded) and resumes evolving on the step after that. A refractory
period of zero steps is allowed. Step counts are pure integers, so no
floating-point rounding can create extra skipped steps.
"""

from __future__ import annotations

import math
from dataclasses import dataclass


@dataclass(frozen=True)
class NeuronConfig:
    """Static parameters of one neuron."""

    tau: float  # membrane time constant, ms (> 0)
    threshold: float  # spike threshold, mV
    reset: float  # reset voltage, mV (< threshold)
    initial: float  # initial voltage, mV (< threshold)
    refractory_steps: int = 0  # full steps skipped after a spike (>= 0)


@dataclass(frozen=True)
class SynapseConfig:
    """A directed, delayed connection between neurons."""

    source: int  # index of the presynaptic neuron
    target: int  # index of the postsynaptic neuron
    weight: float  # signed voltage jump in mV applied on arrival
    delay_steps: int  # integer delay in time steps (>= 1)


@dataclass
class SimulationResult:
    """Recorded output of a simulation run."""

    dt: float
    num_steps: int
    neuron_names: list[str]
    # voltages[step][neuron]: voltage after the update of that step
    # (i.e. after a possible reset).
    voltages: list[list[float]]
    # spike_times[neuron]: list of spike times in ms.
    spike_times: list[list[float]]
    spike_counts: list[int]
    mean_rates_hz: list[float]
    undelivered_spikes: int  # spikes still in transit at simulation end


@dataclass
class Network:
    """A validated LIF network ready to be simulated."""

    dt: float  # time step, ms (> 0)
    num_steps: int  # number of steps to simulate (>= 1)
    neurons: list[NeuronConfig]
    neuron_names: list[str]
    synapses: list[SynapseConfig]
    # equilibrium[step][neuron]: piecewise-constant external balance
    # voltage (mV) valid during that step.
    equilibrium: list[list[float]]

    def run(self) -> SimulationResult:
        """Replay the network and return deterministic recordings."""
        num_neurons = len(self.neurons)
        voltages_now = [n.initial for n in self.neurons]
        # Remaining full steps to skip per neuron (0 means active).
        refractory_left = [0] * num_neurons
        # arrivals[step][neuron]: summed synaptic jumps arriving at the
        # end of that step. Only future steps are ever written.
        arrivals: list[list[float]] = [
            [0.0] * num_neurons for _ in range(self.num_steps)
        ]
        pending_beyond_horizon = 0

        voltages: list[list[float]] = []
        spike_times: list[list[float]] = [[] for _ in range(num_neurons)]

        decay = [math.exp(-self.dt / n.tau) for n in self.neurons]

        for step in range(self.num_steps):
            eq_row = self.equilibrium[step]
            arrival_row = arrivals[step]
            spiked_this_step: list[int] = []

            for i, neuron in enumerate(self.neurons):
                if refractory_left[i] > 0:
                    # Held at reset; arrivals this step are discarded.
                    refractory_left[i] -= 1
                    voltages_now[i] = neuron.reset
                    continue
                # 1) closed-form exponential relaxation toward V_eq
                v = eq_row[i] + (voltages_now[i] - eq_row[i]) * decay[i]
                # 2) add all synaptic jumps arriving at end of this step
                v += arrival_row[i]
                # 3) simultaneous threshold decision + immediate reset
                if v >= neuron.threshold:
                    spiked_this_step.append(i)
                    v = neuron.reset
                    refractory_left[i] = neuron.refractory_steps
                voltages_now[i] = v

            # 4) schedule spikes only after all decisions for this step
            for i in spiked_this_step:
                spike_times[i].append((step + 1) * self.dt)
                for syn in self.synapses:
                    if syn.source != i:
                        continue
                    target_step = step + syn.delay_steps
                    if target_step < self.num_steps:
                        arrivals[target_step][syn.target] += syn.weight
                    else:
                        pending_beyond_horizon += 1

            voltages.append(list(voltages_now))

        spike_counts = [len(times) for times in spike_times]
        duration_s = self.num_steps * self.dt / 1000.0
        mean_rates = [count / duration_s for count in spike_counts]

        return SimulationResult(
            dt=self.dt,
            num_steps=self.num_steps,
            neuron_names=list(self.neuron_names),
            voltages=voltages,
            spike_times=spike_times,
            spike_counts=spike_counts,
            mean_rates_hz=mean_rates,
            undelivered_spikes=pending_beyond_horizon,
        )
