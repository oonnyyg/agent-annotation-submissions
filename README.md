# annual-extremes

Annual-maxima extreme value analysis using only the Python standard library.
Reads a CSV of yearly observations plus a JSON config, fits a maxima Gumbel
distribution, estimates return levels, and quantifies sample-size uncertainty
with a bootstrap.

## Usage

```
python -m annual_extremes config.json
```

Config example:

```json
{
  "input_csv": "observations.csv",
  "output_csv": "annual_series.csv",
  "output_json": "statistics.json",
  "return_periods": [10, 50, 100],
  "bootstrap": {"resamples": 1000, "seed": 42}
}
```

Input CSV columns: `year,value`. Duplicate years keep the maximum value.
Values must be finite. Years missing inside the observed span are listed in
the JSON output (`missing_years`) but are never zero-filled. At least three
valid years are required and the annual maxima must not all be equal.

## Method

- Gumbel (maxima) fit by method of moments:
  - scale = sample standard deviation (denominator n - 1) * sqrt(6) / pi
  - location = mean - Euler-Mascheroni constant * scale
- Return level for return period T > 1 is the quantile at non-exceedance
  probability p = 1 - 1/T:
  `x_T = location - scale * ln(-ln(p))`.
- Empirical exceedance probabilities: sort maxima descending, exceedance
  probability = rank / (n + 1); tied values share their average rank.
- Bootstrap: resample the annual maxima with replacement (original sample
  size), refit, and recompute all return levels per resample. The 95%
  interval uses linear-interpolation percentiles (2.5% and 97.5%) of the
  bootstrap quantiles. Resamples whose values are all equal produce a
  degenerate (constant) quantile and are counted in
  `degenerate_resamples` rather than discarded or redrawn.

## Assumptions and caveats

- Annual maxima are assumed independent and identically distributed (i.i.d.)
  and to follow a Gumbel distribution; the bootstrap additionally assumes the
  sample represents the underlying distribution.
- The bootstrap intervals are pointwise percentile intervals, not a
  simultaneous confidence band across return periods.
- Return levels for long return periods are extrapolations. They are
  estimates under the stated model assumptions, not guarantees or
  predictions of future extremes.

## Tests

```
python -m pytest tests/
```
