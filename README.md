# uncertainty_budget

Correlated measurement uncertainty budget tool. Pure Python standard
library; run as `python -m uncertainty_budget`.

## Usage

```
python -m uncertainty_budget input.json [--json report.json] [--csv budget.csv]
```

Reads a JSON spec of input quantities and result expressions, writes a
JSON report (values, standard uncertainties, result covariance and
correlations) and a CSV uncertainty budget. No code from the input is
ever executed: expressions are parsed by a small recursive-descent
parser, never by `eval`/`exec`.

## Input format

```json
{
  "quantities": [
    {"name": "a", "nominal": 10.0, "std_uncertainty": 0.5},
    {"name": "b", "nominal": 9.0, "std_uncertainty": 0.5}
  ],
  "correlation": [[1.0, -0.9], [-0.9, 1.0]],
  "results": [
    {"name": "difference", "expression": "a - b"},
    {"name": "total", "expression": "a + b"}
  ]
}
```

- Each quantity has a finite `nominal` and a non-negative
  `std_uncertainty`.
- `correlation` is optional; when omitted the inputs are independent.
  When given it must be `n x n`, symmetric, have unit diagonal, entries
  in `[-1, 1]`, and be positive semidefinite as a whole. Individual
  coefficients inside `[-1, 1]` is *not* sufficient — e.g. three
  variables all correlated at `-0.9` is impossible and is rejected.
  Singular (perfectly correlated) matrices are accepted.

## Expressions

Support declared quantity names, numeric constants, parentheses,
`+ - * /`, integer powers `x^k` (integer literal exponent, possibly
negative), and `sqrt(x)`, `exp(x)`, `ln(x)`. Expressions may not
reference undeclared names or other results. Syntax errors, unknown
names, and nominal-point domain errors (division by zero, `sqrt` of a
negative, `ln` of a non-positive value, `0^k` for `k <= 0`) are
reported with the result name and column. Points where the expression
is not differentiable or the derivative is infinite (e.g. `sqrt(0)`)
are rejected, because first-order propagation is meaningless there.

## Method

Sensitivities are exact first-order partial derivatives at the nominal
point, computed with dual numbers. With Jacobian `J` and full input
covariance `Sigma = D R D`, the result covariance is `J Sigma J^T`.
Result standard uncertainties are the square roots of its diagonal;
correlations between results are `cov / (u_r u_s)` and are `null` when
either result has zero uncertainty (no division by zero).

The CSV budget lists, per result, every diagonal term
`J_i^2 Sigma_ii` and every cross term `2 J_i J_j Sigma_ij` with its
sign preserved. The terms sum exactly to the propagated variance —
contributions are never absolutized or renormalized, so cancellation
between correlated error sources stays visible.

### Why error sources can cancel

When two inputs are correlated, part of each one's fluctuation is
shared. If a result depends on them with opposite-sign sensitivities
(e.g. a difference `a - b`), the shared fluctuation affects both
equally and cancels: the cross term `2 J_a J_b cov(a, b)` is negative
when `cov(a, b) > 0`, reducing the total variance below the sum of the
diagonal terms. With perfect negative correlation the same happens to
a sum. Perfect correlation can even drive a result variance to exactly
zero (singular covariance), which this tool accepts and reports.

## Limitations

- This is a **first-order (linearized) propagation** at the nominal
  point. Higher-order effects are ignored, and the output is a
  standard uncertainty, **not** a confidence interval — no coverage
  factor or distribution assumption is applied.
- Matrix checks (symmetry, unit diagonal, positive semidefiniteness
  via an LDL^T pivot test) use a relative tolerance of `1e-9`;
  borderline-indefinite matrices within that tolerance are accepted.
- Sensitivities are computed analytically via dual arithmetic, so no
  finite-difference step-size error is introduced; remaining numerical
  error is ordinary double-precision rounding.

## Development

```
python -m unittest discover -s tests
```
