# Delayed Spiking Network Simulator

A reproducible, standard-library-only Python simulator that replays a leaky
integrate-and-fire (LIF) network on a fixed time grid. It performs no
training: external stimuli and delayed synaptic interactions are played back
step by step to produce deterministic voltage traces and spike records.

## Units

All quantities are unit-less but must be mutually consistent:

- Time: `dt`, `tau`, and stimulus `start`/`end` share one time unit
  (e.g. milliseconds). Synaptic `delay` and `refractory_steps` are integer
  counts of time steps, so they never suffer from floating-point rounding.
- Voltage: `v0`, `reset`, `threshold`, stimulus `v_eq`, and synaptic
  `weight` share one voltage unit (e.g. mV). Weights may be positive
  (excitatory) or negative (inhibitory).
- Reported mean firing rates are in inverse time units (e.g. spikes/ms).

## Configuration (JSON)

```json
{
  "dt": 0.5,
  "steps": 40,
  "neurons": [
    {
      "name": "exc",
      "tau": 10.0,
      "threshold": 1.0,
      "reset": 0.0,
      "v0": 0.0,
      "refractory_steps": 2,
      "stimulus": [{"start": 0.0, "end": 20.0, "v_eq": 2.5}]
    }
  ],
  "connections": [
    {"from": "exc", "to": "exc", "weight": -0.5, "delay": 2}
  ]
}
```

Validation rules:

- `v0 < threshold` and `reset < threshold` for every neuron.
- Stimulus segments are piecewise-constant equilibrium voltages; their
  `start`/`end` times must align with the time grid (exact multiples of
  `dt`), be contiguous and non-overlapping, and together cover the whole
  simulation `[0, steps * dt)`.
- `delay >= 1` (integer steps); self-connections are allowed.

## Update order (per step t)

1. **Evolve**: every non-refractory neuron relaxes in closed form toward
   this step's equilibrium voltage:
   `v <- v_eq + (v - v_eq) * exp(-dt / tau)`.
2. **Deliver**: all synaptic jumps scheduled to arrive at the end of step
   `t` are added to their targets. Jumps arriving at a refractory neuron are
   discarded.
3. **Threshold**: every neuron with `v >= threshold` spikes (at most once
   per step) and resets immediately. Its outgoing jumps are scheduled for
   step `t + delay` — spikes can only affect future steps, so the result
   never depends on neuron iteration order, and all spikes decided in the
   same step are delivered together.

Refractory neurons hold their reset voltage for `refractory_steps` full
subsequent steps (zero allowed), skipping evolution and discarding arrivals.

## Outputs

- Per-step post-reset voltage table (CSV via `--voltages`).
- Spike times (as step indices), per-neuron spike counts, mean firing
  rates, and the number of spikes still in transit when the run ended
  (`undelivered` — reported, not delivered; time is not advanced further).

## Usage

```
python -m dsn examples/two_neurons.json --voltages voltages.csv --summary summary.json
```

## Tests

```
python -m unittest discover -s tests
```

Covers closed-form evolution, refractory skipping, inhibitory connections,
simultaneous same-step arrivals, delayed self-connections, order
independence, undelivered spikes, and configuration validation.
