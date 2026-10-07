# Delayed LIF Spiking Network Simulator

A deterministic, standard-library-only simulator that replays a leaky
integrate-and-fire (LIF) network on a fixed time grid: external
piecewise-constant stimuli plus delayed, signed synaptic interactions.
No training is performed.

## Units

| Quantity            | Unit                |
|---------------------|---------------------|
| Time (`dt`, `duration`, segment bounds) | milliseconds (ms) |
| Voltages (threshold, reset, initial, equilibrium, weights) | millivolts (mV) |
| Time constant `tau` | milliseconds (ms)   |
| Delays, refractory period | whole time steps (integers) |
| Reported mean firing rate | Hz (spikes/second)  |

## Update order (per step `t`, evolving `t -> t + dt`)

1. **Relaxation** — each non-refractory neuron evolves in closed form
   toward this step's equilibrium voltage `V_eq`:
   `V <- V_eq + (V - V_eq) * exp(-dt / tau)`.
   Refractory neurons are held exactly at `reset` and discard any
   arrivals.
2. **Synaptic jumps** — all jumps scheduled to arrive at the end of
   this step are added at once (weights may be positive or negative).
3. **Threshold decision** — made simultaneously for all neurons:
   `V >= threshold` produces exactly one spike and an immediate reset
   to `reset`.
4. **Delivery scheduling** — spikes are scheduled only into future
   steps (`delay_steps >= 1`), so same-step spikes can never influence
   the current step and the result is independent of iteration order.
   Delayed self-connections are allowed.

## Refractory period

After a spike, the neuron skips `refractory_steps` subsequent **full**
steps (held at `reset`, arrivals discarded) and resumes evolving on
the following step. Zero is allowed and means no skipped steps. All
counts are integers, so floating-point rounding can never create extra
skipped steps.

## Configuration (JSON)

```json
{
  "dt": 0.5,
  "duration": 20.0,
  "neurons": [
    {"name": "exc", "tau": 8.0, "threshold": -50.0, "reset": -65.0,
     "initial": -70.0, "refractory_steps": 2}
  ],
  "synapses": [
    {"source": "exc", "target": "exc", "weight": 3.0, "delay_steps": 3}
  ],
  "stimulus": {
    "exc": [[0.0, 8.0, -35.0], [8.0, 20.0, -58.0]]
  }
}
```

Validation rules:

- `initial < threshold` and `reset < threshold` for every neuron.
- `tau > 0`, `dt > 0`, `duration` covers at least one step.
- `delay_steps >= 1`; self-connections (`source == target`) are fine.
- `refractory_steps >= 0` (integer).
- Stimulus segments are `[start_ms, end_ms, equilibrium_mV]`, must
  align to the time grid, must not overlap, and must cover
  `[0, duration)` completely for every neuron.

## Output

- **Voltage table**: post-reset voltage of every neuron after each
  step (`voltages[step][neuron]`).
- **Spike times**: per neuron, in ms (a spike decided on step `t` is
  stamped `(t + 1) * dt`).
- **Counts and mean rates**: per neuron; rate = count / duration.
- **Undelivered spikes**: spikes still in transit when the simulation
  ends are counted as undelivered; time is not advanced further.

## Usage

```
python lif_cli.py example.json                # summary table
python lif_cli.py example.json --voltages     # also print voltage table
python lif_cli.py example.json -o out.json    # write full recordings
```

## Tests

```
python -m unittest test_lif -v
```

Covers closed-form relaxation, one-spike-per-step reset, inhibitory
connections, simultaneous same-step inputs (order independence),
delayed self-connections, refractory skipping with discarded input,
zero refractory period, undelivered-spike counting, and all
configuration validation rules.

## Files

- `lif_sim.py` — simulation core (`Network`, `NeuronConfig`,
  `SynapseConfig`, `SimulationResult`).
- `lif_config.py` — JSON loading and validation.
- `lif_cli.py` — command line interface.
- `test_lif.py` — unit tests (standard library `unittest`).
- `example.json` — runnable two-neuron excitatory/inhibitory example.
