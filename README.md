# dry-friction-motion

Simulates a horizontal slider with dry (Coulomb) friction under a
piecewise-constant external force, using only the Python standard library.

## Usage

```
python -m friction_motion input.json [-o OUTDIR]
```

`input.json` is either a single scenario object or
`{"scenarios": [...]}`. Each scenario is simulated independently from its
own initial state; a scenario that fails validation reports its `reason`
in `results.json` without affecting the others.

Outputs per scenario (`OUTDIR`, default `.`):

- `<name>_trajectory.csv` — sampled `t,x,v` rows at `dt` spacing.
- `<name>_events_energy.json` — events, totals and energy balance.
- `results.json` — index of all scenarios (`ok` / `error` + `reason`).

## Input format

```json
{
  "name": "demo",
  "mass": 1.0,
  "gravity": 10.0,
  "mu_static": 0.5,
  "mu_kinetic": 0.4,
  "x0": 0.0, "v0": 0.0,
  "t_end": 3.0, "dt": 0.25,
  "forces": [
    {"t_start": 0.0, "t_end": 1.0, "value": 10.0},
    {"t_start": 1.0, "t_end": 2.0, "value": -20.0},
    {"t_start": 2.0, "t_end": 3.0, "value": 3.0}
  ]
}
```

## Units

SI throughout: mass kg, gravity m/s², force N, position m, velocity m/s,
time s, energy J. Friction coefficients are dimensionless.

## Model

- Pure horizontal translation; normal force `N = mass * gravity`.
- Requires `mu_static >= mu_kinetic >= 0`, `mass > 0`, `gravity > 0`.
- Force segments are half-open intervals `[t_start, t_end)`, contiguous,
  starting at `t = 0` and ending exactly at `t_end`.
- At rest: if `|F| <= mu_static * N` the slider holds; otherwise it starts
  (kinetic friction applies immediately).
- While sliding, kinetic friction `mu_kinetic * N` opposes the velocity.
  When the velocity reaches zero the slider never crosses through zero
  along its old acceleration; the stick/slip condition is re-evaluated.

## Event ordering

Events are reported as `start`, `stop` (and `hold` at `t = 0` when the
slider begins stuck). If a stop and a force switch happen at the same
instant, the zero-velocity state is reached first (`stop`), then the new
force at that same time decides whether the slider holds or restarts —
possibly in the opposite direction — with no spurious displacement at the
instant itself.

## Computation method

The solution is analytic and event-driven: each force segment is integrated
in closed form, split at force boundaries and at stop events. Sampling
(`dt`) only evaluates this piecewise solution and never changes the
dynamics, so events, distance and energy totals are independent of the
sampling frequency.

Accumulated over each segment:

- `distance` — total path length `∫|v| dt` (never the net displacement).
- `work_force` — work of the external force `∫F dx`.
- `friction_dissipation` — `mu_kinetic * N * distance` (sliding only).
- `energy.residual` — `work_force - friction_dissipation - ΔK`, which is
  zero (up to floating-point roundoff) when the books balance.

## Tests

```
python -m unittest discover -s tests
```

Covers the static-friction threshold (force exactly at `mu_s * N` holds,
just above starts), stop-then-reverse motion, energy-balance residuals,
and independence of results from the sampling step.
