# uncertainty_budget

Uncertainty budget tool for combined measurement results that keeps the effect
of **correlated inputs**. Pure Python standard library; no dependencies.

## Usage

```
python -m uncertainty_budget spec.json [--json out.json] [--csv budget.csv]
```

- `spec.json`: input specification (`-` reads from stdin).
- JSON results go to stdout unless `--json` is given.
- `--csv` writes the per-term variance budget table.

## Input format

```json
{
  "inputs": [
    {"name": "a", "value": 10.0, "std_uncertainty": 0.2},
    {"name": "b", "value": 9.9, "std_uncertainty": 0.2}
  ],
  "correlation": [[1.0, -1.0], [-1.0, 1.0]],
  "results": [
    {"name": "total", "expression": "a + b"},
    {"name": "mean", "expression": "(a + b) / 2"}
  ]
}
```

- Every input needs a nominal `value` and a non-negative `std_uncertainty`.
- `correlation` is optional (defaults to independent inputs). It must be an
  n×n matrix with unit diagonal, symmetric, and positive semidefinite.
  Singular-but-valid matrices (e.g. `[[1, -1], [-1, 1]]`) are accepted.
  Note that every coefficient lying in [-1, 1] is **not** sufficient: the
  matrix as a whole is checked for positive semidefiniteness via a Jacobi
  eigenvalue computation.
- Result expressions may reference declared inputs and numeric constants only
  (never other results). Supported: parentheses, `+ - * /`, integer powers
  `x^2` (including negative exponents), and `sqrt(x)`, `exp(x)`, `ln(x)`.
  Expressions are parsed into an AST by a hand-written parser; nothing in the
  input is ever executed as code. Undeclared names, bad syntax, and nominal
  points outside a function's domain are reported with a column position.

## Method

Results and first-order sensitivities (partial derivatives) are computed at
the nominal point by forward-mode automatic differentiation. The full input
covariance matrix `V = D R D` (standard uncertainties `D`, correlation `R`)
is propagated through the Jacobian `J` as `J V Jᵀ`, giving each result's
variance, standard uncertainty, and the correlations between results. The
correlation of a zero-uncertainty result is reported as `null` (no division
by zero).

The budget lists, per result:

- one **diagonal** term per input: `J_i² V_ii` (always ≥ 0), and
- one **cross** term per input pair: `2 J_i V_ij J_j` (sign preserved).

The terms sum exactly to the propagated variance. Cross terms are **not**
absolute-valued or normalized away: when inputs are (anti)correlated, error
sources can reinforce or **cancel** each other. For example, `a + b` with
correlation −1 has zero total uncertainty — both inputs drift together in
opposite directions and their effects on the sum cancel. Singular correlation
matrices that produce zero total variance are therefore allowed.

Points where an operation is not differentiable or its derivative is infinite
(`sqrt(0)`, `ln(0)`, `1/0`, `0` to a negative power) are rejected.

## Limitations and tolerances

- This is a **first-order (linearized) propagation of uncertainty** (GUM
  Supplement 1 style Monte Carlo would be needed for strongly nonlinear
  models). It does **not** produce confidence/coverage intervals; no
  distribution shape is assumed and no coverage factor is applied.
- Matrix checks use tolerances: symmetry and unit diagonal within 1e-9,
  positive semidefiniteness via Jacobi eigenvalues with a relative threshold
  of 1e-12. Variances that cancel to slightly negative values through
  floating-point roundoff are reported as zero.

## Tests

```
python -m unittest discover -s tests
```
