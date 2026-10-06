# Gray Enclosure Radiation Calculator

Computes net radiative heat exchange between the surfaces of a closed
enclosure of gray, diffuse, opaque surfaces, and solves the equilibrium
temperature of adiabatic (uncontrolled, zero-net-flux) surfaces.
Pure Python standard library — no dependencies.

## Usage

```
python -m enclosure_radiation config.json --csv surfaces.csv --json exchange_details.json
```

Exit codes: `0` all cases solved, `1` at least one case failed (failed
cases are marked `"status": "failed"` in the JSON, never reported as
zero-flux results), `2` invalid input file or configuration.

## Input format

```json
{
  "tolerance": 1e-9,
  "surfaces": [{"name": "heater", "area": 1.0, "emissivity": 0.9}],
  "view_factors": [[0.0, 0.5, 0.5]],
  "cases": [{"name": "steady",
             "temperatures": {"heater": 1000.0, "sink": 300.0, "shield": null}}]
}
```

- `area` in m² (must be > 0), `emissivity` in (0, 1].
- Temperatures in kelvin; `null` marks an adiabatic surface whose
  temperature is solved for.
- Multiple cases share the same geometry and are solved independently.
- The view factor matrix is validated, never silently repaired:
  - all entries non-negative;
  - each row sums to 1 within `tolerance` (summation rule);
  - `A_i F_ij = A_j F_ji` within `tolerance` (reciprocity).
  Violations are rejected with an explicit error.
- Concave surfaces may have a non-zero self view factor `F_ii`.
- Every radiatively connected component of the enclosure must contain at
  least one fixed-temperature surface, otherwise the case is reported as
  under-constrained.

## Governing equations

For each surface `i`, radiosity `J` (leaving flux) and irradiation `G`
(incident flux) in W/m² satisfy

```
G_i = sum_j F_ij J_j
J_i = eps_i sigma T_i^4 + (1 - eps_i) G_i      (fixed temperature)
J_i = G_i                                       (adiabatic, q_i = 0)
```

with `sigma = 5.670374419e-8 W/(m² K⁴)`. These linear equations in `J`
are solved simultaneously by Gaussian elimination with partial
pivoting, so all inter-reflections are fully accounted for (no blackbody
shortcut, no guessed temperatures). Adiabatic temperatures follow from
`T_i = (J_i / sigma)^(1/4)`.

Net heat flux (positive = leaving the surface):

```
q_i = eps_i / (1 - eps_i) * (sigma T_i^4 - J_i)   (eps_i < 1)
q_i = sigma T_i^4 - G_i                            (black surface)
```

Net power exchanged between a pair (antisymmetric by reciprocity):

```
Q_i->j = A_i F_ij (J_i - J_j) = -Q_j->i
```

## Output

- CSV: one row per case and surface — area, emissivity, temperature
  (solved for adiabatic surfaces), radiosity `J`, irradiation `G`, net
  heat flux, net power, and the per-surface balance residual
  `A_i q_i - sum_j Q_i->j`.
- JSON: per case the same surface data plus `pair_exchange_W` (each
  pair's forward/reverse power and its antisymmetry residual) and
  `global_energy_residual_W` (sum of all net powers, ~0 for a closed
  enclosure).

## Accuracy

The geometry tolerance (`tolerance`) controls acceptance of the view
factor matrix. The linear solve is done in double precision; residuals
reported in the outputs are typically at the 1e-10 W level for
well-conditioned problems and can be inspected to judge solution
quality.

## Tests

```
python -m unittest discover -s tests
```

Covers the isothermal limit (zero net flux, adiabatic surface settling
at the common temperature), a two-plate-plus-reradiating-shield case
with energy-balance and antisymmetry checks, concave self-view factors,
disconnected components, input validation, and CLI failure handling.
