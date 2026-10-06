# Pseudorange Position Lab

Offline, pure-stdlib Python solver for **teaching** pseudorange data. It
estimates receiver coordinates and a common clock offset from corrected
satellite positions and pseudoranges, and checks observation sensitivity
via leave-one-out re-solves. It does **not** receive or process real
navigation signals.

## Model

- Predicted pseudorange = geometric range (satellite to receiver) + common
  clock offset (metres).
- Unknowns per epoch: receiver `x, y, z` and clock offset `b` (4 total),
  estimated **jointly** by iteratively reweighted least squares
  (Gauss-Newton). The clock is never fixed to zero to solve position first.
- Weights are `1 / sigma^2` from the input standard deviations. Sigmas are
  used as given — they are never inflated to mask outliers.
- Covariance = inverse of the final weighted normal matrix
  `(A^T W A)^-1`, **not** scaled by the estimated variance factor.

## Robustness

- Every iteration checks the design-matrix rank and the normal-equation
  solve; degenerate geometry raises `RankDeficientError`.
- An initial position coinciding with a satellite raises
  `InitialGeometryError`; non-convergence raises `ConvergenceError`.
  Failures are reported per epoch and never padded with zero increments.
- Epochs are independent: one failing epoch does not block the others.
- With >= 5 satellites, leave-one-out re-solves each drop one satellite,
  always restarting from the epoch's original initial state. The report
  gives the solution shift and the extrapolated residual of the excluded
  satellite. Degenerate subsets are flagged individually; no single
  drop-one result is promoted as "the" answer.

## Inputs

Observations CSV (all lengths in metres, finite values, positive
pseudoranges and sigmas, unique `sat_id` per epoch, >= 4 satellites):

```
epoch,sat_id,x,y,z,pseudorange,sigma
E1,G01,20000000.0,5000000.0,15000000.0,25507442.55,1.0
```

Initial states JSON (clock offset in metres):

```json
{"E1": [0.0, 0.0, 0.0, 0.0]}
```

## Usage

```
python -m pseudorange_lab.cli observations.csv initials.json \
    --json-report report.json --csv-report report.csv
```

Exit code is 0 when every epoch solves, 1 if any epoch failed (failures
are still written to the reports), 2 for input file errors.

The JSON report contains, per epoch: solution (x, y, z, clock_m),
iteration count, per-satellite residuals, weighted residual sum of
squares, covariance matrix with 1-sigma values, and leave-one-out
results. The CSV report is a flat summary with one row per leave-one-out
case.

## Tests

```
python -m unittest discover -s tests
```

Covers recovery of a known position and common clock offset, inverse-
variance weighting, covariance scaling, degenerate geometry, initial
position on a satellite, non-convergence, and leave-one-out behaviour.

## Limitations

This is a teaching tool. The model ignores signal propagation effects
(ionospheric and tropospheric delay, relativity, antenna phase centres),
Earth rotation during signal transit (Sagnac effect), satellite clock and
ephemeris errors, multipath, and any time correlation between epochs.
Input satellite positions and pseudoranges are assumed already corrected.
