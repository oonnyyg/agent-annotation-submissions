# planar-rigid-impact

Event-driven replay tool for 2D rigid-disk collisions inside a
rectangular domain, using only the Python standard library. Disks are
uniform solid cylinders (`I = 1/2 m r^2`); translation and spin exchange
momentum through frictional impacts. Walls are static (infinite mass).

## Usage

```
python -m impact.cli scenario.json --csv traj.csv --diagnostics diag.json
```

Exit codes: `0` completed, `1` invalid input, `2` aborted on ambiguous
simultaneous multi-contact, `3` event limit reached (incomplete).

## Scenario format

```json
{
  "width": 10.0, "height": 10.0,
  "restitution": 0.9,        // 0..1, shared by all contacts
  "friction": 0.4,           // >= 0, Coulomb coefficient
  "t_end": 4.0,
  "sample_dt": 0.25,         // trajectory sampling interval
  "max_events": 1000,        // safety cap; hitting it => incomplete
  "disks": [
    {"x": 3, "y": 4, "vx": 2, "vy": 0, "omega": 1,
     "mass": 1, "radius": 1}
  ]
}
```

Initial disk-disk overlaps and out-of-bounds disks are rejected.

## Model

- Free flight is linear; the next disk-disk time (quadratic in `t`) and
  disk-wall time (linear) are solved analytically. No fixed step, no
  tunnelling-and-rewind.
- Contacts that are touching but not approaching (relative normal
  velocity `>= -APPROACH_TOL`) never fire, so grazing contacts do not
  retrigger.
- Impulses use contact-point relative velocity. Normal impulse:
  `jn = -(1+e) vn / kn`. Tangential impulse first solves for zero slip
  (`jt = -vt / kt`), then is clamped to `|jt| <= mu * jn`. Linear and
  angular velocities are updated together (`dv = J/m`,
  `dw = (r x J)/I`).
- Collisions at the same instant (within `TIME_TOL`) whose disk sets are
  disjoint are resolved independently. If simultaneous contacts share a
  disk, the simulation stops and lists the contact graph instead of
  picking an arbitrary order.
- Samples landing exactly on an event time record the post-collision
  state.

## Output

- CSV: `time,disk,x,y,vx,vy,omega` per sample.
- Diagnostics JSON: per-event impulses (`jn`, `jt`), kinetic-energy
  delta, and momentum delta. For events without wall contact,
  `momentum_check` (|delta p|, should be ~0) is included; with
  `restitution=1, friction=0`, `ke_delta` is ~0.

## Numerical tolerances

Defined in `impact/engine.py`:

- `TIME_TOL = 1e-9` — simultaneity window / minimum future event time.
- `APPROACH_TOL = 1e-9` — threshold on approach speed.
- `GEOM_TOL = 1e-9` — slack in overlap / bounds validation.

## Limitations

- No static / resting contact: persistent contact, stacking, and
  rolling-along-wall equilibria are not modelled. Strongly inelastic
  settings (`e` near 0) can produce event cascades that hit
  `max_events`; the tool then reports the run as incomplete rather than
  guessing a resting state.
- Simultaneous contacts sharing a disk abort instead of being solved as
  a coupled linear system.

## Tests

```
python -m unittest discover -s tests
```

Covers eccentric spin-exchange impacts, wall bounce (elastic, frictional,
inelastic), momentum/energy conservation checks, grazing contacts, input
validation, multi-contact abort, event limit, and post-collision
sampling.
