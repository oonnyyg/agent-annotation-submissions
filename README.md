# binary-meta-analysis

Offline command-line tool that summarizes binary-outcome studies (2x2
counts) with odds-ratio meta-analysis. It reports statistics only; it
does not give decision recommendations. Standard library only.

## Input

CSV with one row per study:

```csv
study_id,e_events,e_nonevents,c_events,c_nonevents
study-1,12,88,20,80
study-2,0,50,3,47
```

- `study_id` must be unique; duplicates are reported with the CSV line number.
- Counts must be non-negative integers; each group total must be positive.
  Violations are reported with the line number and column.

## Usage

```sh
python -m binary_meta studies.csv                 # JSON report to stdout
python -m binary_meta studies.csv --format csv -o report.csv
```

## Methods

- Effect measure: log odds ratio, experimental vs control. Variance on the
  log scale: `1/a + 1/b + 1/c + 1/d`.
- Exclusions: studies with no events in either group, or events in every
  subject of both groups, have an undefined odds ratio and are excluded.
  Excluded studies are listed with reasons and never counted in `n_included`.
- Continuity correction: any other study with a zero cell gets 0.5 added
  to all four cells; otherwise no correction is applied.
- Pooling: inverse-variance fixed effect, and random effects with the
  DerSimonian-Laird between-study variance `tau2` (truncated at 0).
- Heterogeneity: Cochran's Q, I2 (percent), and non-negative `tau2`.
- Intervals: normal-approximation 95% CI on the log scale; odds-ratio
  intervals are obtained by exponentiating the log-scale limits.
- Leave-one-out: each included study is removed in turn and the whole
  analysis (weights, Q, I2, tau2, pooled estimates) is recomputed from the
  remaining studies. With one study left, an effect interval is still
  reported but heterogeneity is marked as not estimable; with none left,
  the result is explicitly empty.

## Limitations

- Normal-approximation intervals can be unreliable for small samples or
  rare events, even with the continuity correction.
- The 0.5 continuity correction can bias estimates when group sizes are
  unbalanced; it is a pragmatic convention, not a remedy for sparse data.
- DerSimonian-Laird `tau2` is a moment estimator and can be imprecise with
  few studies; no Hartung-Knapp adjustment is applied.
- Odds ratios are not risks; with common events they overstate relative
  effects.
- The tool pools whatever is supplied; it does not assess study quality,
  publication bias, or clinical heterogeneity.

## Tests

```sh
python -m unittest discover -s tests
```
