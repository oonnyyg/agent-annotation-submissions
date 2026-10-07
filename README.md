# gapped-allan-stability

Standard-library-only tool that computes the **overlapping Allan deviation**
of fractional-frequency observations on a uniform sampling grid, correctly
handling **gaps** in the record.

## Quantities and units

- Timestamps: seconds, strictly increasing, on a uniform grid with base
  interval `tau0` (declared in the config).
- Values: dimensionless fractional frequency `y(t) = (f(t) - f_nom) / f_nom`.
- Reported `tau = m * tau0` is the averaging time in seconds; `deviation`
  is dimensionless.

## Gap handling

Missing grid points split the record into independent contiguous segments.
No interpolation is done and no averaging window ever crosses a gap. For
each averaging length `m`, every segment contributes its overlapping
squared differences of adjacent length-`m` means; segments are pooled by
**window count**:

```
sigma_y^2(m) = sum_segments sum_i (meanB_i - meanA_i)^2 / (2 * N)
```

where `N` is the total number of valid difference windows. Per-segment
deviations are never averaged directly. Scales without enough data report
`status = "insufficient_data"` with null deviation — never zero or NaN.

## Optional detrending

With `"detrend": true`, an ordinary least squares line is fit to **all**
observations using their actual timestamps (so the time distance across a
gap is preserved, never compressed). The trend is subtracted and the
analysis repeated; the JSON report keeps both the detrended `scales` and
the pre-detrend `raw_scales`, plus the fit coefficients (`slope` per
second, `intercept`). Detrending is off by default.

## Usage

```
python -m gapped_allan data.csv config.json --out-prefix report
```

Input CSV (header required):

```
time,value
0.0,1.2e-9
1.0,1.1e-9
```

Config JSON:

```json
{"tau0": 1.0, "m": [1, 2, 4, 8], "detrend": false}
```

Outputs `report.csv` and `report.json`, sorted by averaging time. If the
grid or config is invalid, the tool exits non-zero **before** writing
anything, so existing reports are never overwritten by a failed run
(reports are also written via a temporary file and atomically renamed).

## Verification

Run the test suite:

```
python -m unittest discover -s tests
```

The tests cover the analytically checkable cases:

- **Constant frequency offset** -> deviation is exactly 0 at all scales.
- **Pure linear drift** `y = d*t` -> adjacent length-`m` means differ by
  exactly `d*m*tau0`, so `sigma_y(m) = d*m*tau0/sqrt(2)`; after detrending
  the deviation collapses to 0 and the fit recovers `d`.
- **Segmented merging** -> windows never cross a gap, and pooled variance
  equals the window-count-weighted sum of per-segment squared differences
  (distinct from averaging per-segment deviations).
- **Failure safety** -> invalid grids/configs leave existing reports
  untouched.
