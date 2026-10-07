# Green-Ampt Rainfall Infiltration Calculator

Replays piecewise-constant rainfall through the Green-Ampt infiltration
model for a homogeneous soil profile, and reports a checkable time series
of infiltration and runoff. Standard library only; Python >= 3.12.

## Usage

```
python -m green_ampt.cli examples/rain.csv examples/soil.json [--tol 1e-9] [--max-iter 200]
```

- `rain.csv`: columns `start,end,rate` — segment start/end in **hours**,
  rainfall intensity in **mm/h**. Segments must be contiguous and ordered;
  overlaps, gaps, negative rates and non-positive durations are rejected.
- `soil.json`: `{"ks": ..., "psi": ..., "delta_theta": ...}` — saturated
  hydraulic conductivity (mm/h), wetting-front suction head (mm), and
  initial moisture deficit (dimensionless, in `(0, 1]`). All must be
  positive.
- `--tol`: bisection tolerance on cumulative infiltration (mm).
- `--max-iter`: iteration cap; failure to converge raises an explicit
  error instead of returning a wrong number.

The output is a process table at every segment boundary and at the
intra-segment ponding (transition) time, followed by totals for rainfall,
infiltration, runoff and the mass-balance residual.

## Model

Cumulative infiltration `F` (mm) starts at zero. The infiltration
capacity is

```
fp(F) = Ks * (1 + psi * dtheta / F)        (F > 0)
fp(0) = infinity                           (limit, no division by zero)
```

Per segment with constant rain rate `i`:

1. **Supply-limited** (`i < fp(F)`): all rain infiltrates, `F` grows at
   rate `i`. If `i > Ks`, capacity eventually drops to `i` at
   `Fp = psi*dtheta / (i/Ks - 1)`; the ponding time inside the segment is
   `tp = t0 + (Fp - F0)/i` and is emitted as a `ponding` event. Rates
   `i <= Ks` never pond.
2. **Capacity-limited (ponded)**: `F` follows the cumulative Green-Ampt
   relation

   ```
   t - t0 = [F - F0 - M * ln((M + F) / (M + F0))] / Ks,   M = psi*dtheta
   ```

   which is monotone in `F`; the segment-end `F` is found by bisection
   with the configured tolerance and iteration cap. Rain exceeding
   capacity becomes runoff **immediately** (no surface storage).
3. **Dry segments** (`rate = 0`): state is frozen; later rain resumes
   from the same cumulative infiltration.

Mass balance closes exactly: total rain = total infiltration + total
runoff. Splitting a constant-rate period into finer segments changes the
result only within solver tolerance (covered by tests).

## Limitations

- Homogeneous soil, sharp wetting front, no redistribution during dry
  periods (cumulative infiltration simply pauses).
- No surface storage/depression retention, no evaporation, no
  interception; excess rain is runoff instantly.
- No layered soils, no water-table rise, no hysteresis.

## Tests

```
python -m unittest discover -s tests
```