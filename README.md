# Retarded Solute Transport (1-D, stdlib only)

Explicit cell-centred finite-volume simulation of a solute pulse in a 1-D
medium with adsorption retardation and first-order decay. Pure Python
standard library — no external numerical packages.

## Model

Unit cross-section and unit porosity. Per cell `i` with size `dx`:

```
R * dc_i/dt = (F_{i-1/2} - F_{i+1/2}) / dx - k * R * c_i
```

- Advective flux `F = v * c_upstream` (upwind, flow in +x direction).
- Diffusive flux `F = -D * (c_{i+1} - c_i) / dx`.
- Left boundary: total input flux `v * c_inlet(t)`.
- Right boundary: zero diffusive flux, advective flux `v * c_last`.
- Retardation `R >= 1` divides the flux divergence (sorption slows transport).
- Decay `k` acts on the **total** inventory `R * dx * c`, not just the
  dissolved part.

## Time stepping

Explicit Euler with automatic sub-stepping. The stable step combines all
process rates (they add, so the sum bounds the step) with a safety factor:

```
dt <= 0.9 / (v/(R*dx) + 2D/(R*dx^2) + k)
```

(only the terms that are non-zero contribute). Steps are shortened so the
integrator lands **exactly** on every inlet-table breakpoint and every
requested output time — pulses are never averaged across their boundaries.
Negative concentrations are never clipped; stability comes from the step
restriction alone.

## Mass balance

The inventory is tracked incrementally with the same fluxes used to update
the state:

```
initial_inventory + cumulative_inflow
    = cumulative_outflow + cumulative_decay + final_inventory
```

`mass_balance.json` reports both sides' components plus the absolute and
relative residual (closes to floating-point roundoff, ~1e-15 relative).
`inventory_from_state` (recomputed as `R * dx * sum(c)`) is included for
cross-checking against the incrementally tracked `final_inventory`.

## Configuration (JSON)

```json
{
  "length": 10.0,               // m, > 0
  "cells": 50,                  // >= 2
  "velocity": 1.0,              // m/s, >= 0
  "dispersion": 0.1,            // m^2/s, >= 0
  "retardation": 2.0,           // >= 1
  "decay": 0.05,                // 1/s, >= 0
  "inlet": [[0.0, 2.0], [1.0, 0.0]],  // piecewise-constant, starts at t=0,
                                      // strictly increasing times, last value held
  "initial": 0.0,               // number or list of `cells` values, >= 0
  "output_times": [0.0, 1.0, 5.0]     // non-negative, increasing
}
```

Concentrations are mg/m^3; all values must be finite.

## Usage

```
python -m retarded_transport.cli config.json \
    --profile profile.csv --outlet outlet.csv --mass mass_balance.json
```

- `profile.csv`: concentration of every cell centre at each output time.
- `outlet.csv`: outlet (last cell) concentration versus time.
- `mass_balance.json`: the discrete mass budget and residual.

## Tests

```
python -m unittest discover -s tests
```

Covers pure advection (retarded front speed `v/R`), zero-velocity diffusion
(mass conserved, solute spreads), an inlet pulse with decay, exact landing
on event times, input validation, and exact mass-budget closure.
