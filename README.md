# wire-field-superposition

Compute the magnetic field of several closed polyline current loops at
probe points, using only the Python standard library. Coils and probe
orientation both affect the reading, so the tool keeps per-coil
contributions, projects the field onto each probe's local axes, and
compares alternative current schemes against a baseline.

## Physical model

- Ideal thin-wire (filament) assumption: each coil is a closed polyline
  of straight segments with no conductor cross-section. Results near a
  wire are not physically meaningful for real conductors.
- Vacuum permeability `mu0 = 4*pi*1e-7 H/m`.
- Each finite straight segment contributes the closed-form Biot-Savart
  field
  `B = mu0*I/(4*pi) * (r1 x r2) * (|r1|+|r2|) / (|r1|*|r2|*(|r1|*|r2| + r1.r2))`,
  where `r1`, `r2` point from the field point to the segment endpoints.
  All segment contributions are summed as vectors; the result is in tesla.
- A point collinear with a segment — including points on the segment's
  extension line — gives `r1 x r2 = 0`, hence exactly zero contribution.
  It is never treated as an infinite straight wire.
- Numerical handling: no softening epsilon is ever added to
  denominators. The only singular case (point on the segment itself,
  where `|r1|*|r2| + r1.r2 = 0`) is excluded by the safety-distance
  check below, so the formula never produces huge pseudo-values.

## Safety distance

If the shortest distance from a probe point to any wire segment is less
than or equal to the positive `--safe-distance` (metres), that point is
marked `computable: false` and the blocking coil names are reported.
Other points are still evaluated. This tool performs an idealised
field calculation only; it does not draw any equipment-safety
conclusions.

## Input formats

### Coils (JSON)

Vertices are ordered along the current direction; the last vertex is
automatically connected back to the first (do not repeat the first
vertex). At least 3 vertices; zero-length edges and non-finite numbers
are rejected. Coordinates in metres, current in signed amperes.

```json
{"coils": [{"name": "loop_a", "current": 4.0,
            "vertices": [[1, 0, 0], [0, 1, 0], [-1, 0, 0], [0, -1, 0]]}]}
```

### Probes (CSV)

One row per probe: position `px,py,pz` (metres) and the world-frame
components of the probe's three local axes `xx..xz, yx..yz, zx..zz`.
The axes must form a right-handed orthonormal basis (tolerance 1e-6).
Local field components are dot products with these axes; rotating the
probe never changes the field magnitude.

```csv
name,px,py,pz,xx,xy,xz,yx,yy,yz,zx,zy,zz
top,0,0,3,1,0,0,0,1,0,0,0,1
```

### Scenarios (JSON, optional)

Independent current schemes over the same geometry. Coils not listed
keep their baseline current; use negative values to reverse a coil.

```json
{"scenarios": [{"name": "b_reversed", "currents": {"loop_b": -4.0}}]}
```

## Usage

```
python -m wirefield.cli --coils examples/coils.json --probes examples/probes.csv \
    --scenarios examples/scenarios.json --safe-distance 0.01 \
    --json-out report.json --csv-out report.csv
```

The JSON report contains, per scenario and probe: per-coil world-frame
contributions, total world vector, total local vector, magnitude, and —
for non-baseline scenarios — the vector difference versus baseline in
both world and local frames (never a magnitude difference). The CSV
report flattens the same data, one row per scenario/probe pair.

## Tests

```
python -m unittest discover -s tests
```

Covers current-reversal cancellation, extension-line zero field,
rotated-probe magnitude invariance, coil/probe validation, safety
distance blocking, and an end-to-end CLI run.
