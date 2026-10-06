# Competitive Binding Equilibrium

Two ligands (A and B) compete for a finite set of independent binding-site
classes. Pure Python standard library; all concentrations in nM.

## Model

Each site class `i` has total concentration `S_i` and dissociation
constants `Kd_ia`, `Kd_ib`. A site binds at most one ligand (1:1 binding,
mutually exclusive occupancy). Given free concentrations `fa`, `fb`:

```
Z_i      = 1 + fa/Kd_ia + fb/Kd_ib
p_free_i = 1 / Z_i
p_a_i    = (fa/Kd_ia) / Z_i
p_b_i    = (fb/Kd_ib) / Z_i
```

Free concentrations are **not** assumed equal to the dosed totals. They are
found by solving the coupled mass-conservation system simultaneously over
all classes (never class-by-class):

```
R_a = fa + sum_i S_i * p_a_i - T_a = 0
R_b = fb + sum_i S_i * p_b_i - T_b = 0
```

The 2x2 system is solved by damped Newton iteration with the analytic
Jacobian; steps are halved to keep `0 <= f <= T` for each ligand. Zero
doses are handled naturally (free = 0). If the iteration does not
converge, a `ConvergenceError` is raised with the current residuals and
no equilibrium result is produced.

## Tolerances

- Equilibrium residual: converged when `|R| <= 1e-12 + 1e-12 * T` (nM)
  for both ligands; max 100 Newton iterations.
- Inverse dose bisection: stops when the bracket width is
  `<= 1e-9 + 1e-10 * dose` nM; the returned dose is the bracket's upper
  edge, so the achieved occupancy is always >= target (up to solver
  tolerance).

## Input formats

Sites JSON (`sites.json`):

```json
{"sites": [{"name": "receptor", "total": 10.0, "kd_a": 1.0, "kd_b": 5.0}]}
```

Experiments CSV: header `total_a,total_b`, one non-negative dose pair per
row. All values must be finite; site totals and Kd values must be > 0.

## Usage

```sh
python -m binding solve sites.json experiments.csv --out results.csv --report report.json
python -m binding invert sites.json --class receptor --target 0.6 --total-b 20 --limit 10000
```

- `solve` prints per-class free/occupied fractions and bound nM, writes a
  results CSV and a JSON mass-balance report (free + bound = total per
  ligand, plus residuals and iteration counts).
- `invert` fixes `total_b` and finds the minimal `total_a` within
  `[0, --limit]` so the named class reaches the target A-occupancy
  (`0 <= target < 1`). If the occupancy at the limit is still below
  target, it reports `unreachable` (exit code 1) and returns no dose —
  the limit is never passed off as a solution.

## Tests

```sh
python -m unittest discover -s tests -v
```

Covers ligand competition, zero dosing, mass conservation, an analytic
single-class cross-check, input validation, and inverse-problem boundary
cases (zero target, unreachable target, minimality).
