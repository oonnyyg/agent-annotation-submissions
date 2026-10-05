# Retarded Solute Transport (1-D)

Pure-stdlib Python simulation of a solute pulse travelling through a 1-D
medium with equilibrium sorption (retardation) and first-order decay.
No external numerical libraries are used.

## Model

Unit cross-section and unit porosity. The governing equation is

    R * dC/dt = -v * dC/dx + D * d2C/dx2 - R * lambda * C

- `C` dissolved concentration [mg/m^3]
- `v` pore-water velocity [m/s], `D` dispersion coefficient [m^2/s]
- `R >= 1` retardation factor, `lambda >= 0` decay rate [1/s]

Decay acts on the **total** (dissolved + sorbed) mass, hence the `R`
factor in the decay term. Stored mass is `R * dx * sum(C)`, not just the
dissolved part.

## Discretisation

- Cell-centred finite volumes on a uniform grid (`cells >= 2`).
- Explicit Euler time integration.
- Advective flux at internal faces: upwind value `v * C_up`.
- Diffusive flux at internal faces: `-D * (C[i+1] - C[i]) / dx`.
- Left boundary: prescribed **total** inflow `v * C_in(t)`.
- Right boundary: zero diffusive flux, advective outflow `v * C_last`.
- Flux divergence is divided by `R` before updating `C`; decay is applied
  to the whole stored inventory.

## Time-stepping strategy

The stable step is the combined explicit limit

    dt <= 1 / (v/(R*dx) + 2*D/(R*dx^2))   and   dt <= 0.02/lambda

Every interval between two "events" (inlet-schedule breakpoints and
requested output times) is subdivided into `ceil(span / dt_max)` **equal**
steps, so integration lands exactly on each event and never averages
across a pulse edge. Negative concentrations are never clipped; stability
is guaranteed by the step-size choice alone.

## Mass balance

Each step accumulates inflow `dt * v * C_in`, outflow `dt * v * C_last`,
and decayed mass `dt * lambda * R * dx * sum(C)`. The solver reports

    initial + inflow - outflow - decayed - stored

which closes to round-off (~1e-16 relative) at every output time.

## Input JSON

```json
{
  "length": 10.0,
  "cells": 100,
  "velocity": 1.0,
  "dispersion": 0.2,
  "retardation": 2.0,
  "decay": 0.02,
  "inlet": [[0.0, 1.0], [2.0, 0.0]],
  "initial": null,
  "output_times": [0.0, 5.0, 10.0, 20.0]
}
```

- `inlet`: piecewise-constant `[time, concentration]` pairs starting at
  `t = 0`, strictly increasing times, non-negative values; the last value
  persists.
- `initial`: optional per-cell non-negative concentrations (default 0).
- All values must be finite; lengths in metres, times in seconds,
  concentrations in mg/m^3.

## Usage

```
python -m solute_transport examples/pulse.json \
    --profiles out/profiles.csv --outlet out/outlet.csv --mass out/mass.json
```

Outputs:

- `profiles.csv` — one row per output time, one column per cell centre.
- `outlet.csv` — breakthrough curve `time, concentration` at the outlet.
- `mass.json` — totals and per-output-time residual history.

## Tests

```
python -m unittest discover -s tests -v
```

Covers pure advection (retarded front speed), zero-velocity diffusion
(mass conservation), decay of the total inventory, and an inlet pulse
(exact landing on breakpoints, non-negative solution, closed budget).
