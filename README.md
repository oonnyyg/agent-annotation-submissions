# Prismatic Hull Hydrostatics

A standard-library-only Python tool that computes the hydrostatic
equilibrium of a prismatic (unit-length) floating body at a list of heel
angles, and reports whether the buoyancy moment about the center of
gravity is righting or upsetting.

## Usage

```
python -m hull_hydrostatics config.json --csv results.csv --diagnostics diagnostics.json
```

### Configuration (JSON)

```json
{
  "section": [[-1, -0.5], [1, -0.5], [1, 0.5], [-1, 0.5]],
  "mass_per_length": 1000.0,
  "fluid_density": 1000.0,
  "gravity": 9.81,
  "center_of_gravity": [0.0, -0.2],
  "angles_deg": [0, 10, -20, 45],
  "moment_tolerance": 1e-9
}
```

- `section`: closed cross-section polygon, a simple (non-self-intersecting)
  polygon without holes. Concave shapes and either vertex winding order are
  accepted; self-intersecting or zero-area polygons are rejected.
- `mass_per_length`: mass per unit length. Must lie strictly between zero
  and the fully submerged displacement mass (`fluid_density * area`).
- `fluid_density`, `gravity`: must be positive.
- `center_of_gravity`: body-frame `[x, y]`.
- `angles_deg`: heel angles in degrees; each angle is solved independently,
  so the sampling order does not affect the physics.
- `moment_tolerance`: moments with absolute value at or below this are
  classified `neutral`. Optional, defaults to `1e-9`.

## Model

- Body coordinates: x horizontal right, y vertical up; positive heel is
  counterclockwise. The section and the center of gravity are rotated
  together, then a vertical translation is found (bisection) so that the
  buoyancy from the submerged area equals the weight. The water surface is
  fixed at world `y = 0`.
- The submerged polygon is computed by clipping the rotated section against
  the half-plane `y <= 0`; its area and centroid (the buoyancy center) are
  computed from the actual clipped polygon, never from the full-section
  centroid.
- The moment about the center of gravity per unit length is
  `M = buoyancy * (Bx - Gx)`, counterclockwise positive. It is classified
  `righting` only when it has the opposite sign to a nonzero heel angle,
  `upsetting` when it has the same sign, and `neutral` when within the
  configured tolerance. `neutral` is **not** a stability claim.

## Outputs

- CSV: one row per angle with `status` (`ok`/`failed`), equilibrium
  `translation`, buoyancy and gravity center positions, `displaced_mass`,
  `buoyancy`, `weight`, equilibrium `residual`, `moment_about_cg`, and
  `classification`. Failed angles are explicitly marked with an `error`.
- Diagnostics JSON: section area/centroid, full displacement mass, target
  displaced area, tolerances, and the full per-angle results.

## Scope and limitations

The model is two-dimensional and static: waves, water ingress (downflooding),
and longitudinal (along-hull) variation are not modeled. A `neutral`
classification means the moment is below tolerance at that angle only; it
says nothing about stability.

## Tests

```
python -m unittest discover -s tests
```
