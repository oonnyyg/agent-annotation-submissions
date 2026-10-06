# Gumbel Annual Maxima

Standard-library-only extreme-value analysis of annual peak records.

## Usage

```
python -m annual_extremes observations.csv config.json series_out.csv stats_out.json
```

- `observations.csv`: header `year,value`; multiple records in one year
  collapse to the annual maximum. All values must be finite. At least
  three distinct years are required, and the annual maxima must not all
  be equal.
- `config.json`:
  ```json
  {"return_periods": [10, 50, 100],
   "bootstrap": {"resamples": 1000, "seed": 42}}
  ```
- `series_out.csv`: annual maxima sorted descending, with average rank
  and empirical exceedance probability.
- `stats_out.json`: fitted parameters, return levels, and bootstrap
  95% intervals. Missing years inside the observed span are listed in
  `missing_years`; they are never filled with zeros.

## Method

**Gumbel (maxima) fit, method of moments.** With sample mean `m` and
sample standard deviation `s` (denominator `n - 1`):

- scale `beta = s * sqrt(6) / pi`
- location `mu = m - gamma * beta`, where `gamma` is the
  Euler–Mascheroni constant (0.57721...)

**Return levels.** For return period `T > 1`, the level is the quantile
at non-exceedance probability `p = 1 - 1/T`:

```
x_T = mu - beta * ln(-ln(1 - 1/T))
```

**Empirical exceedance.** Sorting maxima descending, each value gets
rank `r` (ties share the average rank) and empirical exceedance
probability `r / (n + 1)`.

**Bootstrap uncertainty.** Annual maxima are resampled with replacement
(`n` per replicate, configured count and seed), and the Gumbel fit and
all return levels are recomputed per replicate — every replicate covers
every return period, so input ordering cannot change results. The 95%
interval per return period uses linear-interpolation percentiles
(position `p * (n - 1)`) of the bootstrap levels at 2.5% and 97.5%.
Replicates whose sample is entirely constant are degenerate: the
constant itself is recorded as the quantile, the replicate is counted
in `degenerate_resamples`, and it is neither discarded nor redrawn.

## Assumptions and caveats

- Annual maxima are assumed independent and identically distributed
  (i.i.d.); trends, clustering, or regime shifts violate this.
- The bootstrap intervals are **pointwise** intervals for each return
  period, not a simultaneous confidence band across periods.
- Extrapolation to long return periods is a model-based estimate, not a
  prediction guarantee; uncertainty grows quickly beyond the observed
  record length.

## Tests

```
python -m unittest discover -s tests -v
```
