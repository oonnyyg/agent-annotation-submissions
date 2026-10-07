# planar-rigid-impact

Event-driven replay of 2D rigid disks (uniform solid cylinders) colliding
inside a rectangle, with exchange between translation and spin. Pure
Python standard library, no fixed timestep.

## Usage

```sh
python -m impact.cli scenario.json --csv traj.csv --events events.json \
    --diagnostics diag.json
```

Exit code: `0` simulation completed, `1` event limit reached (not
complete), `2` invalid input or ambiguous simultaneous contacts.

### Input JSON

```json
{
  "width": 10.0, "height": 10.0,
  "restitution": 0.9, "friction": 0.4,
  "t_end": 2.0, "dt_sample": 0.25, "max_events": 1000,
  "disks": [
    {"x": 2, "y": 5, "vx": 2, "vy": 0, "omega": 0,
     "mass": 1, "radius": 1}
  ]
}
```

`restitution` must be in [0, 1], `friction` (Coulomb coefficient mu)
non-negative. Initially overlapping or out-of-bounds disks are rejected.
See `examples/eccentric.json`.

### Outputs

- **CSV trajectory**: rows `t,disk,x,y,vx,vy,omega` at every
  `dt_sample` multiple. A sample landing exactly on an impact uses the
  post-collision state.
- **Events JSON**: one entry per impact time with, per contact, the
  bodies/wall involved, contact normal, normal/tangent impulse, total
  impulse vector, and kinetic energy before/after.
- **Diagnostics JSON**: completion flag, event count, initial/final
  total linear momentum, accumulated wall impulse, initial/final
  kinetic energy, and the largest per-event energy gain (must be ~0).

## Method

- Free flight is linear; the next disk-disk impact solves
  `|p_i - p_j + (v_i - v_j) t| = r_i + r_j` analytically, disk-wall
  times are linear solves. Contacts that touch but do not approach
  (grazing, separating) never fire.
- Impulses use contact-point velocity `v + omega x r`. The normal
  impulse applies restitution `e`; the tangent impulse first computes
  the value that cancels sliding, then is clamped to
  `|J_t| <= mu * J_n`. Linear and angular velocities update together;
  walls have infinite mass.
- Contacts at the same time (within tolerance) that share no disk are
  resolved independently. If simultaneous contacts share a disk, the
  simulation stops and lists the contact graph instead of inventing an
  order.
- Checks: with no wall contact, total linear momentum is invariant;
  with `restitution = 1` and `friction = 0`, kinetic energy is
  conserved (both verified in `tests/`).

## Numerical tolerances

Defined in `impact/core.py`: `TIME_TOL = 1e-9` (simultaneity window),
`GAP_TOL = 1e-9` (initial overlap/boundary slack),
`APPROACH_TOL = 1e-12` (minimum closing speed). These are absolute
tolerances; scale scenarios accordingly.

## Limitations

- **No static/resting contact**: persistent contact, resting stacks,
  and rolling without slip are not modeled. A disk resting exactly on a
  wall with zero approach speed simply never triggers an event.
- Simultaneous multi-point impacts on one shared disk are reported and
  abort the run rather than being ordered arbitrarily.
- `max_events` caps the work; hitting it reports "NOT complete"
  (exit code 1) instead of silently truncating.

## Tests

```sh
python -m unittest discover -s tests
```
