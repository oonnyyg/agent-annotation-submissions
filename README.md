# rectangular-point-pattern

Ripley K/L analysis for point patterns in a rectangular study region,
using only the Python standard library. Distinguishes edge-induced bias
from genuine clustering signals via translation edge correction and a
reproducible Monte Carlo control. No plots are generated; outputs are
CSV and JSON.

## Usage

```
python -m point_pattern points.csv config.json [--out-dir OUT]
```

- `points.csv`: header row with `x,y` columns; coordinates use the
  lower-left corner as origin in the half-open rectangle
  `[0, width) x [0, height)`. At least two distinct points are required;
  out-of-bounds and duplicate locations are rejected.
- `config.json`:
  - `width`, `height` (required, positive): study region dimensions.
  - `n_radii` (default 50): number of grid points, or `radii`: explicit
    list. The grid is strictly increasing, starts at 0, and ends at most
    at half the shorter side. The rectangle is not treated as periodic.
  - `simulations` (default 199): positive integer number of Monte Carlo
    simulations.
  - `seed` (default 12345): fixed integer seed for reproducibility.

Outputs in `--out-dir` (default `.`):

- `k_l_curves.csv`: per radius, the cumulative ordered-pair count,
  uncorrected K, translation-corrected K, `L = sqrt(K/pi)`, and `L - r`.
- `simulation_test.json`: Monte Carlo test report (see below).

## Method

- All ordered pairs of distinct points are accumulated by distance;
  pairs at distance exactly `r` are included. Counts are ordered-pair
  counts (`n*(n-1)` maximum), never mixed with unordered counts.
- Translation edge correction: each ordered pair with displacement
  `(dx, dy)` gets weight `area / overlap`, where `overlap` is the area
  of the intersection of the window with its translate by `(dx, dy)`,
  i.e. `(width - |dx|) * (height - |dy|)`. Then
  `K(r) = area / (n*(n-1)) * sum of weights` over pairs within `r`.
  The uncorrected K uses weight 1 for every pair.
- Monte Carlo control: `simulations` configurations of the same number
  of points are drawn independently and uniformly in the same rectangle
  with a fixed seed. The test statistic is the maximum over the whole
  radius grid of `|L(r) - r|`. The reported p-value is the plus-one
  corrected upper-tail probability `(exceedances + 1) / (simulations + 1)`,
  with ties counted toward the tail.
- The JSON also lists the pointwise min/max envelope of simulated L
  curves. This is **not** a simultaneous confidence band: it only
  bounds each radius separately.

## Assumptions and limitations

- The null model is complete spatial randomness: points are i.i.d.
  uniform over the rectangle. Rejection indicates deviation from this
  specific model (clustering, regularity, or inhomogeneity), not a
  particular alternative.
- The Monte Carlo p-value has finite-sample resolution: with `s`
  simulations the smallest attainable p-value is `1/(s+1)`, and the
  estimate carries binomial sampling error. Increase `simulations` for
  sharper conclusions.
- The statistic aggregates over the whole radius grid, so the test
  controls the grid-wide deviation; the pointwise envelope does not.

## Tests

```
python -m unittest discover -s tests
```
