# Dry-Friction Stick-Slip Motion

Simulates a horizontal block under a piecewise-constant external force with
Coulomb dry friction (static / kinetic), producing sampled trajectories,
stick/slip events, and a work-energy balance check. Pure Python standard
library, no dependencies.

## Usage

```
python -m friction_motion scenarios.json [-o OUT_DIR]
```

The input is a single scenario object, a list, or `{"scenarios": [...]}`.
Each scenario is simulated independently from its own initial state; a
scenario that fails validation is reported with its reason in
`summary.json` and does not affect the others.

Outputs per scenario (`OUT_DIR`, default `friction_out/`):

- `<name>_trajectory.csv` — columns `t, x, v, moving, distance, work_ext,
  dissipation` sampled every `sample_dt`.
- `<name>_events.json` — event list plus the energy summary.
- `summary.json` — status (or error reason) and energy totals per scenario.

## Input schema

| field | unit | meaning |
|---|---|---|
| `mass` | kg | block mass (> 0) |
| `gravity` | m/s^2 | gravitational acceleration (> 0); normal force N = m*g |
| `mu_static` | - | static friction coefficient, >= `mu_kinetic`, >= 0 |
| `mu_kinetic` | - | kinetic friction coefficient, >= 0 |
| `x0`, `v0` | m, m/s | initial position / velocity (default 0) |
| `t_end` | s | simulation end (> 0) |
| `sample_dt` | s | output sampling interval (> 0) |
| `force_segments` | - | `[[t_start, force], ...]` or objects with `start`/`force` (N) |

Force segments must start at t = 0, have strictly increasing start times,
and continuously cover `[0, t_end]`. Each segment is left-closed,
right-open: a force written at `t_start` takes effect exactly at that
instant.

All quantities are SI: metres, seconds, kilograms, newtons, joules.

## Model and event ordering

- Only horizontal translation; the normal force is `m * g`.
- At rest, the block stays stuck while `|F| <= mu_static * N`; it starts
  moving only when the force exceeds the maximum static friction, in the
  direction of the force.
- While sliding, kinetic friction `mu_kinetic * N` opposes the current
  velocity (at a start from rest it opposes the driving force).
- When the velocity reaches zero the block does **not** coast through the
  zero point under its old acceleration. Instead, at that instant:
  1. a `stop` event is recorded (zero-velocity state reached first);
  2. the force in effect at that instant — per the left-closed segments,
     so a force switch at the same timestamp uses the *new* force — is
     compared against `mu_static * N`;
  3. the block either sticks (`stick`) or restarts, possibly in the
     opposite direction (`start_moving`), with no fabricated instantaneous
     displacement.
- `force_switch` events are recorded whenever a new segment begins.

## Computation method

Because the force is piecewise constant, acceleration is constant between
events, so each phase is integrated analytically
(`x = x0 + v0*dt + a*dt^2/2`). Sampling only evaluates this closed-form
solution at the requested times — the simulation itself never depends on
`sample_dt`, so positions, velocities, and energy totals are identical for
any sampling frequency.

Accumulated over the run:

- `distance_total` — total path length (sum of `|dx|`), never the net
  displacement; both are reported for comparison.
- `work_external` — integral of `F dx` over the actual motion.
- `friction_dissipation` — integral of `mu_kinetic * N * |dx|`.
- `residual = work_external - friction_dissipation - delta_kinetic_energy`,
  which is zero up to floating-point roundoff, verifying self-consistency
  of external work, frictional dissipation, and kinetic-energy change.

## Tests

```
python -m unittest discover -s tests -v
```

Covers the static-friction threshold (exactly at / just above
`mu_static * N`), stop-then-reverse and stop-then-stick behaviour, a stop
landing exactly on a force switch, work-energy balance, sampling-frequency
independence, and batch runs with a failing scenario.

See `examples/scenarios.json` for a runnable input.
