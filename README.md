Compound Gear Kinematics Checker
================================

A standard-library-only tool that checks compound gear trains: it decides
from the connection graph whether every shaft speed is determined, solves
the linear speed constraints per operating case, and audits the signed
power balance for the given external torques.

Usage
-----

    python -m gear_kinematics INPUT.json [-o SPEEDS.csv] [-d DIAG.json]

* `SPEEDS.csv` (default `speeds.csv`): columns `case,shaft,speed_rpm`.
  Speeds are only written for cases with a unique solution; otherwise the
  cell is left empty.
* `DIAG.json` (default `diagnostics.json`): per-case status, constraint
  residuals, free shafts or conflict sources, and the power audit.

Run the tests with `python -m unittest discover -s tests`; an example
input lives in `examples/planetary.json`.

Input format
------------

```json
{
  "shafts": ["in", "mid", "out"],
  "connections": [
    {"type": "external_mesh", "label": "m1",
     "gear1": {"shaft": "in", "teeth": 20},
     "gear2": {"shaft": "mid", "teeth": 40}},
    {"type": "rigid", "shafts": ["mid", "out"]},
    {"type": "internal_mesh",
     "gear1": {"shaft": "a", "teeth": 15},
     "gear2": {"shaft": "b", "teeth": 60}},
    {"type": "planetary",
     "sun": {"shaft": "s", "teeth": 30},
     "ring": {"shaft": "r", "teeth": 90},
     "carrier": "c"}
  ],
  "cases": [
    {"name": "run",
     "speeds": {"in": 200},
     "fixed": ["other"],
     "torques": {"in": 10.0, "out": -400.0}}
  ]
}
```

Connection types
----------------

* `rigid` — two shafts on the same axis rotate together: `w_a - w_b = 0`.
* `external_mesh` — external gears, opposite directions: `z1*w1 + z2*w2 = 0`.
* `internal_mesh` — pinion inside a ring, same direction:
  `z1*w1 - z2*w2 = 0`.
* `planetary` — simple planet set (sun, ring, carrier). The Willis
  equation `z_s*(w_s - w_c) + z_r*(w_r - w_c) = 0` constrains the three
  members. Teeth must be positive integers and the geometry must allow
  `z_ring = z_sun + 2*z_planet` with a positive integer planet tooth
  count; anything else is rejected at load time.

Sign and unit conventions
-------------------------

* All shafts share one global positive direction; mesh signs encode the
  relative direction, so a negative speed simply means "opposite the
  chosen positive direction".
* Input speeds are revolutions per minute; output CSV keeps rpm.
* Torques are N*m with the same sign convention as speeds.
* Power is `P = T * w` with `w = rpm * 2*pi/60` (rad/s), reported in
  watts. Positive power means the external torque drives the train.
  Missing torques count as zero. Internal reaction torques and efficiency
  losses are not modelled.

Solving semantics
-----------------

* Each case is solved independently; cases never influence each other.
* All kinematic and case constraints form one linear system solved by
  Gaussian elimination (tolerance `1e-9` for pivoting and residuals).
  The connection order in the file does not affect the physical solution.
* `unique`: speeds and per-constraint residuals are reported, and the
  power audit is computed.
* `underdetermined`: the number of remaining degrees of freedom and the
  free shafts are reported; free shafts are never silently set to zero
  and no power is emitted.
* `conflicting`: the sources (connection labels and case entries)
  involved in the contradiction are listed; no speeds or power are
  emitted.
* Redundant but consistent constraints are accepted.
