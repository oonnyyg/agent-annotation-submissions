# Shaded PV String Calculator

A small standard-library-only tool that finds the **global maximum power
point (MPP)** of a series PV string whose modules have mismatched
characteristics (e.g. partial shading), and reports which modules are
bypassed by their diodes at the optimum.

## Model (simplified, not a physical prediction)

Each module is a **piecewise-linear** V(I) curve defined by:

- `voc` — open-circuit voltage (V, > 0)
- `isc` — short-circuit current (A, > 0)
- `vknee`, `iknee` — knee point, strictly inside (0, voc) × (0, isc)
- `diode_drop` — bypass diode forward drop (V, > 0; default 0.5)

The curve is a straight line from (0, voc) to (iknee, vknee), then a
straight line from (iknee, vknee) to (isc, 0). Beyond `isc` the second
segment is extrapolated and the module voltage is clamped at
`-diode_drop` (module bypassed). Weak modules are **never removed** and
the string current is **never capped** at the smallest `isc`.

This is a simplified educational model. It does **not** model inverters,
temperature, irradiance-dependent voltage, or real diode physics, and is
not an accurate prediction of real hardware.

## Exact global MPP (not a grid search)

String voltage V(I) is the sum of module voltages, hence piecewise
linear; power P(I) = I·V(I) is piecewise quadratic. The solver collects
every module breakpoint (knee, short-circuit, bypass-switching current),
splits the current axis there, and checks each segment's interior
quadratic vertex **and** its endpoints, restricted to I ≥ 0 and
V_string ≥ 0. Equal-power peaks are resolved toward the smaller current.
The CSV sampling `step` therefore cannot change the reported optimum.

## Usage

```
python -m pv_string examples/input.json --csv-dir out --report out/report.json
```

Input JSON (see `examples/input.json`):

- `step` — CSV sweep sampling step in amps (output resolution only)
- `diode_drop` — default diode drop; per-module `diode_drop` overrides
- `conditions` — list of operating conditions to compare; each has a
  `name` and an ordered `modules` list (`voc`, `isc`, `vknee`, `iknee`)

Outputs:

- One CSV per condition (`current_a, voltage_v, power_w`) in `--csv-dir`
- A JSON report with, per condition: MPP current/voltage/power and the
  per-module voltage and `bypassed` flag at the optimum, in the original
  module order. A module exactly at the clamp voltage counts as bypassed.

## Tests

```
python -m unittest discover -s tests
```

Covers identical modules, weak-module bypass, and the local-peak case
where a naive scan would lock onto the wrong hump.
