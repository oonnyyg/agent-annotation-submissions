# serial-arm-kinematics

Offline kinematics checker for serial robot arms (Go standard library only).
Given a joint-space sample it computes the tool pose and instantaneous tool
velocity. It does **not** do inverse kinematics or collision planning.

## Model

- 1 to 6 joints, each `revolute` or `prismatic`, standard Denavit–Hartenberg
  convention. Each joint transform is applied in the order
  `RotZ(theta) * TransZ(d) * TransX(a) * RotX(alpha)`.
- For a revolute joint the variable is `theta = theta0 + q + offset`; for a
  prismatic joint it is `d = d0 + q + offset`.
- All units are metres and radians. Every sample is checked against the
  per-joint `limit` range; out-of-limit samples are rejected with an explicit
  error and produce no pose output.
- Fixed `base` and `tool` rigid transforms are configurable. Their rotation
  matrices are validated for orthonormality and determinant +1. The tool
  transform is composed after the last joint frame, so the tool offset is
  rotated by the end-effector orientation (it is not simply added in world
  coordinates).

## Velocity

The 6xN geometric Jacobian of the tool origin is expressed in the **world
frame**: rows 1-3 are linear velocity (m/s), rows 4-6 angular velocity
(rad/s). Revolute columns are `Jv = z x (p_tool - o)`, `Jw = z`; prismatic
columns are `Jv = z`, `Jw = 0`. Because `p_tool` is the tool origin, a fixed
tool offset changes the linear-velocity rows. The tool twist is `J * qd`.

## Numerical check

The linear-velocity rows are verified against finite differences of the tool
origin position, and the per-column error is reported. Central differences
are used in the interior; at a joint limit a legal one-sided (forward or
backward) difference is used so no out-of-limit pose is ever evaluated.

## Input format

Chain file (see `examples/chain.json`):

```json
{
  "joints": [
    {"name": "j1", "type": "revolute", "theta": 0, "d": 0.34, "a": 0,
     "alpha": -1.5707963267948966, "offset": 0, "limit": [-3.14, 3.14]}
  ],
  "base": {"rotation": [[1,0,0],[0,1,0],[0,0,1]], "translation": [0,0,0.05]},
  "tool": {"rotation": [[1,0,0],[0,0,-1],[0,1,0]], "translation": [0,0,0.12]}
}
```

Samples file (see `examples/samples.json`):

```json
{"samples": [{"name": "reach", "q": [0.5], "qd": [0.1]}]}
```

## Output

- JSON detail (`-json-out`): per sample the joint frames, tool pose, 6xN
  Jacobian (column-major list), tool twist, and per-column numerical-check
  errors. Rejected samples contain only `name`, `ok: false` and `error`.
- CSV summary (`-csv-out`): one row per sample with status, tool position,
  linear/angular velocity and the maximum Jacobian check error.

## Run

```sh
go run . -chain examples/chain.json -samples examples/samples.json \
  -json-out examples/report.json -csv-out examples/summary.csv
```

## Test

```sh
go test ./...
```

Tests cover mixed revolute/prismatic chains, non-identity base transforms,
tool offsets under end-effector rotation, limit rejection, rotation-matrix
validation, and one-sided differences at joint limits.
