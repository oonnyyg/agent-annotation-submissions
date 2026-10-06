# rainflow-fatigue-audit

Pure standard-library Python tool that converts a local stress time history
into rainflow cycles and accumulated fatigue damage, keeping the provenance
(original endpoint times) of every cycle. A single exceedance peak is **not**
equated with a fatigue-life conclusion; damage is only accumulated from
counted cycles against an S-N model.

## Usage

```sh
python -m rainflow_audit.cli history.csv model.json \
    --cycles-out cycles.csv --summary-out summary.json
```

- `history.csv`: two columns `time,stress` (MPa). Times must be strictly
  increasing, stresses finite. An optional header row is allowed.
- `model.json`: S-N model, e.g.

```json
{
  "reference_stress": 100.0,
  "exponent": 5.0,
  "mean_correction": "goodman",
  "ultimate_strength": 300.0
}
```

`reference_stress` and `exponent` must be positive. `mean_correction` is
`"none"` (default) or `"goodman"`; Goodman requires a positive
`ultimate_strength`.

Outputs:

- `--cycles-out`: per-cycle CSV with range, mean, amplitude, half/full count,
  equivalent amplitude, life, damage contribution, and the original endpoint
  times of each cycle.
- `--summary-out`: JSON summary with per-cycle entries and the total damage
  accumulated over the whole record (never per-segment sums added together).
  Without `--summary-out` the summary is printed to stdout.

## Method

1. **Turning points**: consecutive equal stresses keep only the last point;
   then the first/last points and every direction reversal are kept. A
   constant record yields zero cycles and zero damage.
2. **Three-point stack rainflow**: if the new range is smaller than the
   previous range, the point is pushed; otherwise the previous range is
   closed. With exactly three points on the stack a half cycle is counted and
   the oldest point removed; with more than three a full cycle is counted and
   both ends of the previous range are removed, then the comparison repeats.
   After the input is exhausted, each adjacent residual pair counts as a half
   cycle — residual half cycles are neither dropped nor doubled.
3. **Damage**: amplitude is half the range. Life is
   `(reference_stress / equivalent_amplitude) ** exponent`. With Goodman
   correction the equivalent amplitude is
   `amplitude / (1 - max(mean, 0) / ultimate_strength)`. Zero-range cycles
   carry no damage. If the Goodman denominator is non-positive the cycle is
   marked not evaluable, contributes nothing, and the summary sets
   `damage_complete: false` with a warning — the total is then a lower bound,
   never disguised as a normal value.

## Limitations

- The record is finite: residual half cycles reflect unclosed reversals at
  the record boundaries, so totals depend on where the record starts and
  ends. Longer or repeated records may count differently.
- Rainflow counting plus a linear (Miner-style) S-N accumulation ignores
  load-sequence effects, mean-stress effects beyond the optional Goodman
  correction, notch/plasticity effects, and scatter in the S-N curve itself.
- The S-N model is a simple power law with no endurance limit or knee; a
  counted cycle does not by itself prove or disprove fatigue failure.

## Tests

```sh
python -m unittest discover
```
