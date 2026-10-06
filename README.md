# wire-field-superposition

Compute the magnetic field of several closed polyline current loops at probe
points, using only the Python standard library. Geometry is fixed per run;
independent current scenarios (amplitude changes or reversal) are compared
against a baseline as **vector** differences in world and probe-local
coordinates.

## Physical model and assumptions

- **Ideal thin-wire filament**: each coil is a closed polygon of straight
  current filaments with no cross-section. The last vertex is implicitly
  connected back to the first.
- Vacuum permeability `mu0 = 4*pi*1e-7 H/m`; no materials, no magnetics.
- Each straight segment contributes the exact Biot-Savart closed form for a
  finite filament:

  ```
  B = mu0*I/(4*pi*h^2) * (r1.u/|r1| - r2.u/|r2|) * (u x d)
  ```

  with unit tangent `u`, endpoint vectors `r1 = p-a`, `r2 = p-b`, and `d` the
  perpendicular offset of the point from the segment line (`h = |d|`).
  Contributions are superposed vectorially; output is in tesla.
- **Collinear points contribute zero.** A point exactly on the segment line
  (including the extension beyond the endpoints) gives `d = 0`, hence zero
  field. It is *not* approximated as an infinite straight wire.
- **Safety distance**: a probe point whose minimum distance to any wire
  segment is `<= --safe-distance` is reported as not computable, naming the
  offending coil(s); all other points are still evaluated. The code never
  divides by a tiny stand-in denominator to fabricate a huge value.

## Probe orientation

The probe CSV gives each point and the probe's three local axes in world
coordinates. Axes must form a right-handed orthonormal basis (unit length,
mutually orthogonal, `u x v = w`, tolerance 1e-6). Local components are plain
dot products `B_local_i = B_world . axis_i`, so rotating the probe cannot
change the field magnitude.

## Input formats

### Coil config (JSON)

```json
{
  "coils": [
    {"id": "loop_a",
     "vertices": [[-0.1, -0.1, 0.0], [0.1, -0.1, 0.0], [0.1, 0.1, 0.0], [-0.1, 0.1, 0.0]],
     "current": 5.0}
  ],
  "scenarios": [
    {"name": "oppose", "currents": {"loop_a": -5.0}}
  ]
}
```

- Vertices are metres, ordered along the current direction; do not repeat the
  first vertex at the end. At least 3 vertices; zero-length edges (including
  the closing edge) and non-finite numbers are rejected.
- `current` is signed amperes; negative reverses the direction.
- `scenarios` is optional. Each scenario must give a finite current for every
  coil; geometry never changes between scenarios.

### Probe points (CSV)

Header: `x,y,z,u1,u2,u3,v1,v2,v3,w1,w2,w3` — position (m) and the three local
axes in world coordinates. See `examples/probes.csv`.

## Usage

```
python -m wirefield examples/coils.json examples/probes.csv \
    --safe-distance 0.005 --json-out out/report.json --csv-out out/report.csv
```

Without `--json-out`/`--csv-out` the JSON report goes to stdout. Output
directories are created as needed.

## Report contents

- Per point: `computable` flag and `blocked_by` coil list when within the
  safety distance.
- Per computable point, for the baseline and each scenario: every coil's
  contribution (`per_coil`), the world total, and the probe-local total, each
  with components and magnitude.
- Per scenario: `diff_world_vs_baseline` and `diff_local_vs_baseline` —
  component-wise vector differences plus the norm of the difference vector.
  This is deliberately not the difference of magnitudes.

## Tests

```
python -m unittest discover -s tests
```

Covers reversal cancellation of two coincident loops, zero field on segment
extension lines, rotation invariance of the probe-local magnitude, the
infinite-wire limit of a long segment, square-loop centre field, safety
distance gating, and input validation.

## Scope note

This tool reports computed field values only. It does not assess equipment or
personnel safety and draws no conclusions from the numbers.
