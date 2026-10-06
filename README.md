# gaussian-plume-exposure

Offline multi-source exposure accounting with a steady-state Gaussian plume
model. Compares receptor concentrations under different wind conditions and
keeps each source's contribution to every result.

**Scope:** screening-level estimates for one inert or first-order-decay
substance. Results are **not** a regulatory compliance determination.

## Usage

```
python -m plume_exposure SCENARIO.json MET.csv \
    --contributions contributions.csv --summary summary.json
```

- `SCENARIO.json`: sources, receptors, dispersion power law, decay rate.
- `MET.csv`: meteorological periods (`start,end,wind_from_deg,wind_speed`).
- `contributions.csv`: one row per (period, receptor, source) with the
  concentration attributable to that source.
- `summary.json`: per-receptor time integral, duration-weighted average,
  and peak concentration with its period (earliest period wins ties).

See `examples/` for a runnable sample.

## Model

- Flat-terrain steady-state Gaussian plume with full ground reflection
  (image source). No plume rise, no terrain effects.
- Wind direction is the direction the wind comes **from**, degrees clockwise
  from true north. Receptor offsets are rotated into downwind/crosswind
  coordinates; receptors not downwind of a source get zero contribution.
- Dispersion widths follow power laws in downwind distance `x` (metres):
  `sigma_y = lateral_coeff * x**lateral_exponent`,
  `sigma_z = vertical_coeff * x**vertical_exponent`.
  All coefficients and exponents must be positive.
- Concentration: `Q / (2*pi*u*sigma_y*sigma_z)` times lateral, vertical
  (direct + reflected), and first-order decay `exp(-k * x / u)` factors,
  where `k` is the decay rate (0 = inert).
- Each met period is treated as an independent steady state; source
  contributions are summed per receptor. Receptor averages are weighted by
  valid period durations; gaps between periods are excluded (not counted as
  zero-concentration time).

## Units

| Quantity            | Unit                          |
| ------------------- | ----------------------------- |
| Coordinates, heights| metres (east/north, non-negative heights) |
| Emission rate       | mass per second (non-negative)|
| Wind speed          | metres per second (must be > 0) |
| Decay rate          | 1/second (non-negative)       |
| Concentration       | mass per cubic metre          |
| Time integral       | concentration * seconds       |

Keep the mass unit consistent (e.g. g/s in gives g/m3 out).

## Met period rules

Periods are left-closed/right-open intervals `[start, end)`. Gaps are
allowed; overlaps, non-positive wind speeds, negative emission or decay
rates, and non-positive dispersion coefficients/exponents reject the whole
run with an error — calm periods are never silently replaced by zeros.

## Limitations

- Steady-state assumption breaks down near the source and during calm or
  rapidly shifting winds; near-source (tens of metres) estimates are
  unreliable because sigma power laws extrapolate poorly as `x -> 0`.
- No plume rise, building downwash, terrain, or deposition.
- Single substance only; no chemistry beyond first-order decay.

## Tests

```
python -m unittest discover -s tests
```
