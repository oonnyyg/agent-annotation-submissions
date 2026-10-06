# catenary-clearance

Local checking tool for overhead cable spans. Given two support points and a
cable length, it recovers the exact catenary shape, computes support
tensions, and finds the minimum vertical clearance to a terrain polyline.
Pure Python standard library; 2D self-weight statics only.

## Usage

```
python -m catenary_clearance config.json [--out-dir DIR] [--samples N]
```

Writes `report.json` (extrema and pass/fail per case) and one
`samples_<case>.csv` per solved case into the output directory (default:
the input file's directory).

## Input format

```json
{
  "supports": {"left": {"x": 0, "y": 30}, "right": {"x": 120, "y": 34}},
  "linear_density": 2.2,
  "gravity": 9.81,
  "terrain": [[-5, 0], [40, 6], [80, 2], [125, 8]],
  "max_tension": 6000,
  "min_clearance": 3.0,
  "samples": 241,
  "cases": [{"name": "medium", "length": 135}]
}
```

- `supports`: right `x` must be strictly greater than left `x`; heights may differ.
- `linear_density` (kg/m) and `gravity` (m/s^2) must be positive.
- `terrain`: `[x, y]` points, `x` strictly increasing, covering the whole span.
- `cases`: each `length` must be strictly greater than the support chord
  distance; an unsolvable case is reported with an error and does not
  affect the other cases.
- `max_tension`, `min_clearance`: optional limits checked per case.
- `samples`: CSV row count only. All extrema are computed analytically, so
  changing the sample density never changes the reported extremes or the
  pass/fail verdicts.

See `examples/example.json`.

## Model

- Uniform, inextensible cable; weight per unit length `w = linear_density * gravity`.
- Shape `y(x) = a*cosh((x - xv)/a) + k` (true catenary, not a parabola).
- Horizontal tension `H = w*a`; support tensions `T = H*cosh((x - xv)/a)`.
  The report includes the endpoint-height and arc-length residuals of the
  solution.
- The mathematical vertex may lie outside the span (steep, near-taut
  cables); it is reported as-is, and the lowest point **within** the span
  is reported separately.
- Clearance is the vertical gap `y_cable(x) - y_terrain(x)`. On each linear
  terrain segment the gap has a single stationary point
  `x* = xv + a*asinh(slope)`; checking it plus all segment endpoints gives
  the exact minimum. A negative clearance is kept as its true value and
  flagged `"collision": true`.

## Units and tolerances

Any consistent unit set works (e.g. m, kg/m, m/s^2 -> tensions in N).
The catenary parameter is solved by bisection to a relative tolerance of
1e-13; residuals in the report show the achieved accuracy. Near-taut
(`length` barely above the chord) and deeply sagged cables are both
handled by the bracketing solver; failures raise a descriptive error
instead of returning a wrong shape.

## Tests

```
python -m unittest discover -s tests
```
