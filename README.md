# circular-mixture-fit

Fit a 1–3 component **von Mises mixture** to weighted circular observations
using weighted EM. Standard library only (Python ≥ 3.12). Directions are
treated on the circle — observations straddling 0°/360° form one continuous
cluster; no linear-distance clustering is used anywhere.

## Usage

```
python -m circular_mixture.cli OBSERVATIONS.csv CONFIG.json \
    [--assignments assignments.csv] [--report report.json]
```

Exit codes: `0` converged, `1` ran to the iteration cap without converging
(outputs are still written, clearly marked), `2` invalid input.

## Inputs

### Observations CSV

Header columns `angle,weight`. Angles are in the unit declared by the config
(`"degrees"` or `"radians"`); any finite value is accepted and normalized to
one turn (angles differing by whole turns are the same direction). Weights
must be positive and finite. Non-finite values (`nan`, `inf`) are rejected.

### Config JSON

```json
{
  "unit": "degrees",
  "tolerance": 1e-8,
  "max_iterations": 500,
  "components": [
    {"weight": 0.5, "mu": 355.0, "kappa": 5.0},
    {"weight": 0.5, "mu": 185.0, "kappa": 5.0}
  ]
}
```

- `components`: 1 to 3 initial components. `mu` in the declared unit,
  `kappa >= 0` (0 = uniform), `weight >= 0`; weights are renormalized to
  sum to one, and at least one must be positive.
- `tolerance`: relative log-likelihood change below which EM stops.
- `max_iterations`: hard cap; hitting it is **not** convergence.

The number of components is fixed by the config — it is not selected
automatically.

## Outputs

- **Assignments CSV**: one row per observation with the original angle, the
  normalized radian angle, the weight, one responsibility column per
  component, and the MAP component assignment. Components are sorted by
  normalized direction and the probability columns are reordered to match.
- **Report JSON**: `converged` flag, human-readable `message`, iteration
  count, final weighted log-likelihood, the full per-iteration
  log-likelihood history, and the fitted components (weight, mu in radians
  and degrees, kappa). Non-converged runs are marked `"status": "NOT
  CONVERGED"`.

## Method and numerical notes

- **Model**: mixture of von Mises densities
  `f(θ) = exp(κ cos(θ−μ)) / (2π I₀(κ))`; `κ = 0` is treated exactly as the
  uniform distribution `1/(2π)`.
- **E-step**: responsibilities are computed in log space with the
  log-sum-exp trick, so they always sum to 1 without overflow, even at
  κ = 50.
- **M-step**: component means use the weighted **circular** mean (vector
  sum of unit vectors), so samples on either side of 0° average correctly
  instead of being pulled to 180°. κ is updated from the mean resultant
  length R via the Banerjee et al. approximation and clamped to
  `[0, 50]`. Component weights are the normalized weighted responsibility
  masses and stay non-negative.
- **Bessel functions**: `I₀`, `I₁` are evaluated by their power series,
  which is accurate to machine precision for κ ≤ 50.
- **Degenerate cases**: a component whose weighted responsibility mass
  collapses below `1e-12` of the total is reset to a uniform component
  (κ = 0) with a floor weight of `1e-12`, keeping it recoverable and all
  outputs finite. A non-finite log-likelihood aborts the fit with
  `converged: false` rather than reporting success.
- **Convergence**: declared only when the relative change of the weighted
  log-likelihood drops below `tolerance`. Exhausting `max_iterations`
  yields exit code 1 and a `NOT CONVERGED` report — never a silent success.

## Tests

```
python -m unittest discover -s tests
```

Covers wrap-around clusters, uniform data (κ → 0), weight handling and
weight/duplication equivalence, input validation, empty components, and
the non-convergence path. See `examples/` for a runnable dataset.
