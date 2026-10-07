# circular-mixture-fit

Weighted von Mises mixture fitting for directional (circular) observations,
using only the Python standard library. Observations that straddle the
zero-degree direction (e.g. 359 deg and 1 deg) are treated as one continuous
directional cluster — no linear-distance clustering is used anywhere.

## Usage

```
python -m circular_mixture_fit.cli --data data.csv --config config.json \
    --assignments assignments.csv --report report.json
```

(With the package installed, the `circular-mixture-fit` console script is
equivalent.) See `examples/` for a runnable input pair.

## Input CSV

Header columns `angle,weight`. One observation per row.

- `angle`: direction in the unit declared by the config (`deg` or `rad`).
  Must be finite. Angles are normalized to one turn, so 359 deg, -1 deg and
  719 deg are the same direction.
- `weight`: positive, finite sample weight. Non-positive or non-finite
  values are rejected with an error.

## Configuration JSON

```json
{
  "unit": "deg",
  "components": [
    {"weight": 0.6, "mu": 10.0, "kappa": 3.0},
    {"weight": 0.4, "mu": 170.0, "kappa": 3.0}
  ],
  "tolerance": 1e-10,
  "max_iterations": 500
}
```

- `unit`: `"deg"` (default) or `"rad"`; applies to the CSV angles, the
  initial `mu` values, and the angles in the outputs.
- `components`: 1 to 3 initial components. Each needs `weight` (>= 0, not
  all zero; renormalized to sum to one), `mu` (initial direction, in `unit`),
  and `kappa` (initial concentration, 0 to 50 inclusive). `kappa = 0` means
  the uniform distribution on the circle.
- `tolerance`: relative log-likelihood change below which EM stops
  (default 1e-8).
- `max_iterations`: iteration cap (default 500). Reaching the cap is
  reported as **not converged**, never as success.

The number of components is fixed by the config; it is not selected
automatically.

## Model and numerics

- Mixture of von Mises densities
  `f(theta) = exp(kappa * cos(theta - mu)) / (2*pi*I_0(kappa))`, fitted with
  weighted EM. `kappa = 0` is handled exactly as the uniform density
  `1/(2*pi)`.
- The M-step direction update is the weighted **circular** mean
  (`atan2` of summed sines/cosines), so clusters crossing 0 deg keep their
  true direction instead of being averaged toward 180 deg.
- Concentration uses the Banerjee et al. rational approximation to
  `A^{-1}(R)` refined by Newton steps, clamped to `[0, 50]`.
- `I_0` and `A(kappa) = I_1/I_0` use power series with an asymptotic branch
  for large kappa; densities are evaluated in log-space and responsibilities
  are normalized with the log-sum-exp trick, so there is no exponential
  overflow and each row of responsibilities sums to one.
- Component weights stay non-negative and are renormalized to sum to one
  after every M-step.
- Degenerate handling: a component whose effective sample count falls below
  1e-12 is kept in the output with weight 0 and a uniform density (kappa 0)
  rather than being silently dropped. Non-finite log-likelihoods or failed
  responsibility normalization abort the run with an error (exit code 1);
  failures are never masked with infinities or a fake success status.

## Outputs

- Assignment CSV: `index,angle,weight,component_1_probability,...` with one
  responsibility column per component. Angles are echoed in the configured
  unit.
- Report JSON: `converged`, `status`, `iterations`, `log_likelihood`,
  `log_likelihood_history` (one entry per EM sweep, including the initial
  evaluation), and the fitted `components`.
- Components are sorted by normalized direction `mu` in both outputs, and
  the responsibility columns follow the same order. Responsibilities and
  the total log-likelihood are recomputed once with the final parameters.
- If EM exhausts `max_iterations`, the report has `"converged": false` and
  a `not converged` status string, and the CLI prints a warning.

## Tests

```
PYTHONPATH=src python -m unittest discover -s tests
```
