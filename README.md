# laminate_response

Small-deformation classical laminate theory (CLT) analysis with uniform
thermal loading. Pure Python standard library; no GUI, no finite elements.

## Usage

```
python -m laminate_response input.json [-o result.json] [--csv stresses.csv]
```

- `input.json`: materials, layup, and any number of independent load cases
  (see `examples/laminate.json`).
- Result JSON: ABD matrix, thermal resultants per unit temperature rise, and
  per-case midplane strains, curvatures, layer surface stresses, and
  equilibrium residuals.
- CSV: stress at the bottom and top surface of every layer, in both global
  and material axes. Stresses on the two sides of an interface are reported
  separately and are generally discontinuous.

Each load case is solved independently; no state carries over between cases.

## Input format

```json
{
  "materials": {
    "name": {"e1": ..., "e2": ..., "nu12": ..., "g12": ...,
             "alpha1": ..., "alpha2": ...}
  },
  "layup": [{"material": "name", "thickness": ..., "angle_deg": ...}],
  "load_cases": [{"name": "...", "forces": [Nx, Ny, Nxy],
                  "moments": [Mx, My, Mxy], "delta_t": ...}]
}
```

Validation rejects non-positive moduli or thickness, non-positive-definite
lamina stiffness (`1 - nu12*nu21 <= 0`), and singular ABD systems.

## Units and sign conventions

Any consistent unit system works; typical choice:

- Moduli in Pa, thickness in m, CTE in 1/K, temperature difference in K.
- Force resultants `Nx, Ny, Nxy` in N/m; moment resultants `Mx, My, Mxy`
  in N.
- Stresses in Pa; midplane strains dimensionless (engineering shear
  strain); curvatures in 1/m.

Conventions:

- Layers are listed from bottom (most negative z) to top; the midplane
  `z = 0` is at the center of the total thickness.
- Ply angle is measured counterclockwise from the global x axis to the
  material 1-axis (fiber direction).
- Strain through the thickness: `eps(z) = eps0 + z * kappa`.
- Positive curvature produces positive (tensile) strain at positive z.
- `delta_t` is the uniform temperature change relative to the stress-free
  temperature; positive means heating.
- Mechanical stress is `sigma = Qbar * (eps0 + z*kappa - alpha * delta_t)`;
  the free thermal strain is subtracted before recovering stresses.
- Extension-bending coupling (B matrix) of unsymmetric layups is fully
  included in the 6x6 solve.

## Verification

The resultants obtained by integrating the recovered layer stresses through
the thickness are compared with the applied loads in every case; the
difference is reported as `equilibrium_residual` (should be at round-off
level).

Run the test suite:

```
python -m unittest discover -s tests
```

Tests cover symmetric layups (zero B, no bending under in-plane load),
free thermal deformation (flat symmetric, curved unsymmetric), reversed
layup order (A and D unchanged, B and curvature sign flipped), and
equilibrium residual checks.
