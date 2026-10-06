# Rectangular Point Pattern Analysis

Ripley K/L analysis of point locations recorded in a rectangular study
plot, separating edge-induced bias from genuine clustering signals via a
reproducible Monte Carlo control.  Pure Python standard library; no
plots are produced.

## Usage

```
python -m point_pattern POINTS.csv CONFIG.json [OUT_DIR]
```

- `POINTS.csv`: two numeric columns `x,y` (optional header row).
- `CONFIG.json`: see below.
- `OUT_DIR`: output directory (created if missing; default `.`).

Outputs:

- `kl_curves.csv` — columns `radius`, `ordered_pair_count`,
  `k_uncorrected`, `k_translation_corrected`, `l`.
- `simulation_test.json` — Monte Carlo test report and pointwise
  envelope of simulated L curves.

## Configuration

```json
{
  "width": 10.0,
  "height": 10.0,
  "n_radii": 11,
  "simulations": 99,
  "seed": 12345
}
```

- `width`, `height`: positive plot dimensions.  The window is the
  half-open rectangle `[0, width) x [0, height)` with the origin at the
  lower-left corner; the boundary is **not** wrapped into a torus.
- Radius grid: either explicit `radii` (strictly increasing, starting at
  0, not exceeding half the shorter side) or `n_radii` (evenly spaced
  grid from 0 to half the shorter side).
- `simulations`: positive integer number of simulated configurations.
- `seed`: fixed integer seed for the random number generator.

Input validation rejects non-positive window sides, out-of-window
points, duplicate locations, fewer than two points, and malformed radius
grids.

## Estimators

All ordered pairs `(i, j)` with `i != j` are accumulated by distance;
pairs at distance exactly equal to a radius are included.  The reported
pair counts are therefore ordered counts (each unordered pair appears
twice) and are never mixed with unordered counts.

For each radius `r`:

- Uncorrected: `K_raw(r) = A * count(r) / (n * (n - 1))`.
- Translation (Ohser–Stoyan) edge correction: each pair with
  displacement `(dx, dy)` gets weight `A / |W ∩ (W + (dx, dy))|`, i.e.
  the plot area divided by the overlap area of the window and its
  translate by the pair displacement.  Then
  `K(r) = A * sum(weights of pairs with d <= r) / (n * (n - 1))`.
- `L(r) = sqrt(K(r) / pi)`.

## Monte Carlo test

Under the null hypothesis of complete spatial randomness (CSR), the
same number of points is placed independently and uniformly at random
inside the same rectangle, `simulations` times, using the fixed `seed`
(results are bit-for-bit reproducible for a given seed).

The test statistic is the maximum absolute deviation of `L(r) - r`
over the entire radius grid.  The reported upper-tail probability uses
the plus-one correction

```
p = (1 + #{simulated stats >= observed stat}) / (1 + simulations)
```

with ties counted towards the tail.

The JSON report also lists the pointwise minimum and maximum envelope
of the simulated L curves.  This envelope is **not** a simultaneous
confidence band: at any single radius it covers the simulated range,
but the probability that a CSR curve stays inside it everywhere is not
controlled.

## Assumptions and limitations

- The null model assumes independent, uniformly distributed points
  (homogeneous CSR) within the rectangle; deviations may reflect
  clustering, regularity, or inhomogeneity of the intensity.
- With few points or few simulations the Monte Carlo p-value is coarse:
  its finest resolution is `1 / (simulations + 1)`, and edge-corrected
  K estimates at radii near half the shorter side rely on few pairs
  with large weights, so they are high-variance.

## Development

```
python -m unittest discover -s tests
```

Tests cover translation edge-correction weights, inclusive radius
endpoints, ordered-pair counting, input validation, and seed
reproducibility.  Example inputs live in `examples/`.
