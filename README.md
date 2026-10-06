# Competitive Binding Equilibrium

Pure-stdlib Python tool for two ligands (A and B) competing for a finite
set of independent binding-site classes. All concentrations are in nM.

## Model and assumptions

- **1:1 binding**: each site binds at most one ligand molecule; a site is
  either empty, bound to A, or bound to B (mutually exclusive).
- For site class `i` with total `S_i` and dissociation constants
  `Kd_A,i`, `Kd_B,i`, the statistical weights of the three states are
  `1`, `a / Kd_A,i`, `b / Kd_B,i` (where `a`, `b` are the *free*
  concentrations). State probabilities are these weights normalised by
  their sum `1 + a/Kd_A,i + b/Kd_B,i`.
- Free concentrations are **not** assumed equal to the dosed totals. They
  are found by solving the joint mass-conservation equations
  `A_tot = a + sum_i boundA_i(a, b)` and likewise for B, across **all**
  site classes simultaneously. Site classes are never solved independently
  and summed.

## Solver and tolerances

- Nested monotone bisection: the outer loop brackets free `b` in
  `[0, B_tot]`, the inner loop solves free `a` in `[0, A_tot]` for that
  `b`. Both residual functions are strictly monotone, so the unique root
  is always bracketed (zero doses included).
- Convergence is declared when the mass-balance residuals are within
  `tol` (default `1e-10` nM, adjustable via `--tol`). After the loop the
  residuals and the bounds `0 <= free <= total` are re-checked; on
  failure a `SolverError` reports the cause and both residuals, and no
  unconverged iterate is returned as an equilibrium.
- The inverse problem (minimum total A to reach a target occupancy of A
  on a chosen class, with total B fixed) exploits monotonicity of
  occupancy in total A and bisects `[0, limit_a]`. If the occupancy at
  `limit_a` still falls short (beyond `occ_tol = 1e-9`), the result is
  reported as a failure with the achieved occupancy; the limit is never
  returned as if it were a solution.

## Input formats

`sites.json`:

```json
{
  "site_classes": [
    {"name": "tight", "total": 10, "kd_a": 1, "kd_b": 5},
    {"name": "loose", "total": 50, "kd_a": 100, "kd_b": 2}
  ]
}
```

`mixtures.csv` (one row per experiment, non-negative totals in nM):

```csv
total_a,total_b
30,40
0,0
```

All numeric inputs must be finite; site totals and Kd values must be
positive, ligand totals non-negative. Invalid input aborts with an error.

## Usage

Solve equilibria for a batch of mixtures:

```sh
python -m competitive_binding solve sites.json mixtures.csv \
    --csv-out results.csv --report report.json
```

Inverse dose finding (minimum total A for 50% occupancy of class
`tight`, with B fixed at 10 nM and A limited to 1000 nM):

```sh
python -m competitive_binding inverse sites.json --class tight \
    --target 0.5 --total-b 10 --limit-a 1000 --report inverse.json
```

Outputs:

- `results.csv`: per mixture and site class - free concentrations, empty
  and bound amounts, and per-class occupancies of A and B.
- JSON report: full mass-balance budget per mixture (free, bound,
  residuals, iteration count) plus per-class states; for the inverse
  command, success flag, minimum total A, achieved occupancy, and the
  equilibrium at the solution. Exit code is `1` when the inverse target
  is unreachable within the limit, `2` on input or solver errors.

## Tests

```sh
python -m unittest discover -s tests -v
```

Covers ligand competition, zero dosing, joint-vs-independent solving,
input validation, non-convergence reporting, and inverse boundary cases
(zero target, unreachable target, minimality of the dose).
