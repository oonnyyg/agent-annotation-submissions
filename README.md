# Particle Settling Column

Still-water settling-column analysis tool written with the Python standard
library only. It computes the terminal settling velocity of each particle
size class, predicts mean mass concentrations in sampling layers, and checks
the suspended/deposited mass balance. Flocculation and particle interactions
are not modelled.

## Model assumptions

- Quiescent (still) water column of positive depth `H` and constant
  cross-sectional area `A`.
- Spherical, non-interacting particles; no flocculation, no hindered settling.
- Each size class is initially uniformly distributed over the full depth.
- The acceleration phase is neglected: particles instantly move at their
  terminal velocity and deposit immediately on reaching the bottom.
- All quantities are finite SI values: metres, kilograms, seconds.

## Terminal velocity

The terminal velocity `v` balances submerged weight against drag:

```
0.5 * rho_f * Cd(Re) * A_p * v^2 = (rho_p - rho_f) * g * V_p
Re = rho_f * v * d / mu
Cd = 24/Re + 3/sqrt(Re) + 0.34
```

The drag side is strictly increasing in `v`, so a bisection on an expanding
bracket finds the unique positive root. The solve reports the Reynolds
number and the relative force-balance residual; if the residual cannot be
reduced below 1e-12 within the iteration limit, a `ConvergenceError` is
raised and no prediction is produced. The low-Reynolds Stokes formula is
never applied directly.

## Sampling concentrations

At time `t` the suspended region of a class is `[v*t, H]`. The mean mass
concentration in a sampling interval `[z_top, z_bottom]` (measured downward
from the surface, inside the column, positive thickness) is computed from
the overlap length between the suspended region and the interval — not from
whether the interval centre contains particles.

## Usage

```
python -m settling scenario.json samples.csv --out-prefix result
```

Scenario JSON:

```json
{
  "column": {"depth": 2.0, "area": 0.05},
  "fluid": {"density": 998.2, "viscosity": 1.002e-3, "gravity": 9.81},
  "classes": [
    {"diameter": 1.0e-4, "density": 2650.0, "mass": 0.02},
    {"diameter": 2.0e-5, "density": 2650.0, "mass": 0.01}
  ]
}
```

Sampling CSV (`samples.csv`) with header `time,z_top,z_bottom` and
non-negative times; intervals must lie within the water column.

Outputs (prefix `result` in the example):

- `result_velocities.csv` — per-class diameter, density, terminal velocity,
  Reynolds number, drag coefficient and force-balance relative residual.
- `result_concentrations.csv` — per-request mean concentration of each
  class in the sampling interval [kg/m^3].
- `result_mass_balance.csv` — per-class and total suspended/deposited mass
  at each requested time; suspended + deposited equals the initial mass.

## Tests

```
python -m unittest discover -s tests -v
```

Covers the force-balance residual, the fine-particle (Stokes) limit,
partial clear-water layers, fully settled classes, overlap-based
concentrations, mass conservation and input validation.
