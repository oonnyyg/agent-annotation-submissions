# fourbar-motion-audit

Planar four-bar linkage position and velocity calculator.  Pure Python
standard library; no dynamics, no animation.

## Usage

```
python -m fourbar input.json -o out_prefix
```

Writes `out_prefix.csv` (one row per input angle) and
`out_prefix.diagnostics.json` (run summary, residual maxima, issue list).

## Mechanism model

- Fixed pivots: `A = (0, 0)` and `D = (r1, 0)` (ground link `r1` on the x-axis).
- Input pivot: `B = A + r2*(cos theta, sin theta)`, input angle `theta`
  measured counterclockwise, in **radians**.
- Output pivot `C`: intersection of the coupler circle `circle(B, r3)`
  and the output circle `circle(D, r4)`.
- All four link lengths must be positive finite numbers.

## Input JSON

```json
{
  "links": {"r1": 4.0, "r2": 1.0, "r3": 3.0, "r4": 2.5},
  "branch": 1,
  "angles": [0.0, 0.1, 0.2],
  "omegas": [1.0, 1.0, 1.0],
  "probe": {"u": 1.5, "v": 0.5}
}
```

- `angles`: input angles in radians; `omegas`: input angular velocities
  in rad/s, same length as `angles`.
- `branch`: `+1` or `-1`.  The assembly branch is the sign of the 2D
  cross product `cross(D - B, C - B)`.  The chosen sign is kept for the
  whole sweep; the tool never silently switches branch or falls back to
  a nearest-point solution.
- `probe` (optional): point in coupler-local coordinates — origin at `B`,
  x-axis along `B -> C`, y-axis rotated +90 degrees.  Its position and
  velocity are reported per angle.

## Units

Angles in radians, angular velocities in rad/s.  Lengths and velocities
use any consistent unit system (e.g. m and m/s).

## Status values and singularity criterion

Let `d = |D - B|`, `a = (d^2 + r3^2 - r4^2) / (2d)`, `h^2 = r3^2 - a^2`,
and `tol = 1e-9 * max(r3, r4, d)^2`.

- `ok` — `h^2 > tol`: two intersections, the one matching `branch` is used.
  Position, velocities (`vC`, `omega3`, `omega4`), probe data and
  residuals are all reported.
- `singular` — `0 <= h^2 <= tol` (circles tangent, including `d = r3 + r4`
  and `d = |r3 - r4|`): the configuration is unique and kept, but the
  velocity is undetermined, so all velocity fields are left empty.  No
  large finite placeholder values are emitted.
- `unreachable` — `h^2 < -tol` (circles separate or nested): no assembly
  exists for this angle.  Negative radicands are never forced through an
  absolute value.
- `non_unique` — `d = 0` and `r3 = r4`: the circles coincide and the
  configuration is not unique.

## Velocity solution

Differentiating the loop-closure constraints gives the 2x2 linear system

```
(C - B) . (vC - vB) = 0
(C - D) . vC        = 0
```

solved in closed form for `vC`.  Follower angular velocities come from
`omega = cross(r, v_rel) / |r|^2` for the coupler (`omega3`) and output
link (`omega4`).  Every `ok` row includes residuals: length residuals
`|C-B| - r3`, `|C-D| - r4` and velocity residuals equal to the left-hand
sides above; both should be at round-off level.

## Tests

```
python -m unittest discover -s tests
```

Covers both assembly branches, branch-sign preservation across a sweep,
tangent (singular) configurations, unreachable and coincident-circle
cases, finite-difference velocity checks, and the probe point.
