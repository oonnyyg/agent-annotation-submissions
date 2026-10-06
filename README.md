
# Aquifer Pumping Response Calculator

Predicts drawdown at observation points from multiple pumping/injection
wells in an ideal confined aquifer, and compares the recovery phase after
pump shutdown. Pure Python standard library — no external hydrogeology
packages.

## Usage

```
python -m aquifer_response config.json [--csv out.csv] [--report report.json]
```

See `examples/example_config.json` for a complete input. Run the tests
with `python -m unittest discover -s tests`.

## Model assumptions

- Homogeneous, isotropic, confined aquifer of infinite areal extent and
  constant thickness; Theis (transient, no leakage) solution applies.
- Uniform metre/second units: transmissivity T in m^2/s, storativity S
  dimensionless, rates in m^3/s, times in seconds, drawdown in metres.
- Each well's rate is piecewise constant; the rate before the first
  schedule entry is zero. Positive rates pump, negative rates inject.
- Responses of rate increments superpose in time and space. A rate change
  contributes exactly zero at its switch instant. Stopping a well is a
  negative increment added to the ongoing response — the pumping history
  is never erased, which is what produces the recovery curve.
- Drawdown is positive when the hydraulic head drops; injection yields
  negative drawdown.

## Well function

`W(u)` (the exponential integral `E1(u)`, `u = r^2 S / 4 T t`) is
evaluated with a power series for small `u` and a convergent continued
fraction (modified Lentz algorithm) for large `u`, accurate to machine
precision from `u ~ 1e-8` to underflow at `u ~ 745` (where `W(u)`
returns exactly 0). Accuracy is controlled by `well_function.tolerance`
and `well_function.max_terms` in the config; failures raise an error
stating the reason and abort the run without writing outputs.
Evaluation is stateless, so repeated queries or extra sample times never
change values at previously computed times.

## Input JSON

- `aquifer`: `transmissivity`, `storativity` (both must be positive).
- `well_function` (optional): `tolerance`, `max_terms`.
- `wells`: `name`, `x`, `y`, `schedule` — list of `[time_s, rate_m3s]`
  pairs with strictly increasing times.
- `observation_points`: `name`, `x`, `y` — must not coincide with a well.
- `times`: strictly increasing output times (s).
- `observed` (optional): measured drawdowns per point with `times`,
  `drawdowns`, and strictly positive `weights` of equal length.
- `candidates` (optional): finite list of `{name, transmissivity,
  storativity}` parameter sets to score against `observed`.

## Outputs

- CSV: one row per observation point and time with the total drawdown
  and each well's individual contribution.
- JSON report: aquifer parameters, well-function settings and
  convergence status, per-well contribution series, total drawdown
  series, and — when candidates and observations are given — per-series
  residuals, weighted squared error, weighted RMS, and the candidate
  ranking. Candidates are evaluated exactly as given (no continuous
  optimisation); ties keep the input order. If any computation fails,
  existing output files are left untouched.
