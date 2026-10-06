# rainflow-fatigue-audit

Standard-library-only tool that converts a local stress time history into
fatigue cycles and accumulated damage, keeping the provenance (original
endpoint times) of every cycle.

## Usage

```
python -m rainflow_fatigue.cli history.csv config.json \
    --cycles-out cycles.csv --summary-out summary.json
```

- `history.csv`: columns `time,stress`. Time must be strictly increasing,
  stress is a finite signed value in MPa.
- `config.json`:

```json
{
  "reference_stress": 200.0,
  "exponent": 3.0,
  "mean_correction": "goodman",
  "ultimate_strength": 500.0
}
```

`mean_correction` is `"none"` or `"goodman"`. `reference_stress` and
`exponent` must be positive; Goodman requires a positive
`ultimate_strength`.

## Method

1. Turning points: consecutive equal stresses keep only the last point;
   first/last points and direction reversals are kept. A constant record
   yields zero cycles and zero damage.
2. Three-point stack rainflow: if the new range X is smaller than the
   previous range Y, keep pushing; otherwise close a cycle on Y. With
   exactly three stacked points it counts as a half cycle (oldest point
   removed); with more it counts as a full cycle (both endpoints of Y
   removed). Leftover adjacent pairs each count as a half cycle —
   residuals are never dropped or doubled.
3. Each cycle records range, mean, half/full count, amplitude = range/2,
   and the original endpoint times. Zero-range cycles contribute no
   damage.
4. Life per cycle: `N = (reference_stress / equivalent_amplitude) ** exponent`.
   Goodman: `equivalent_amplitude = amplitude / (1 - max(mean, 0) / Su)`.
   If the denominator is non-positive the cycle is flagged not evaluable
   and the total damage is reported as `null` rather than a fake number.
5. Damage accumulates as count / life over the whole record at once —
   never segmented and re-summed.

## Limitations

- The record is finite: residual half cycles reflect the unclosed
  history, not a prediction of future loading. A single peak exceeding
  a limit is a static/event check, not a fatigue-life conclusion.
- The S-N model is a simple power law with optional Goodman mean
  correction; it ignores sequence effects, notch plasticity, and
  scatter, and is not a substitute for a certified fatigue assessment.
- Damage totals near or above 1 indicate the model's assumed life is
  consumed under these assumptions only.

## Tests

```
python -m unittest discover -s tests
```
