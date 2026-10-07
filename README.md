# serial-arm-kinematics

Offline forward-kinematics checker for serial arms (Go standard library
only). Given a joint-space sample it computes the tool pose and the
instantaneous tool velocity, and verifies the position Jacobian against a
finite-difference reference. No inverse kinematics, no collision or path
planning.

## Model conventions

- 1 to 6 joints, each `revolute` or `prismatic`, standard DH parameters.
- Per-joint transform order: **Rz(theta) -> Tz(d) -> Tx(a) -> Rx(alpha)**.
  The joint variable is `theta` for revolute joints and `d` for prismatic
  joints; the other three DH fields are fixed.
- Units are metres and radians (velocities: m/s and rad/s).
- Each joint has a zero-position `offset` added to the sample variable, and
  travel limits `[min, max]` checked against `position + offset`. Samples
  outside the limits are rejected and produce no pose.
- `base` and `tool` are fixed 4x4 rigid transforms. Both are validated:
  orthonormal rotation, determinant +1, bottom row `[0 0 0 1]`. The tool
  transform is applied on the last joint frame (`T_tool = T_last * tool`),
  so the tool offset rotates with the end effector; it is never added in
  world axes.

## Velocity and Jacobian

The geometric Jacobian of the tool origin is expressed in the **world
frame**: rows 0-2 are linear velocity (m/s), rows 3-5 angular velocity
(rad/s). For a revolute joint the column is `[z x (p_tool - p); z]`; for a
prismatic joint it is `[z; 0]`, where `z` and `p` are the joint's parent
frame axis and origin in world coordinates. Because `p_tool` includes the
fixed tool transform, a tool offset changes the linear velocity of revolute
columns. Tool velocity is `J * qdot`.

The position rows of the Jacobian are checked numerically per column
(central difference, step 1e-6, tolerance 1e-4). At a travel limit a legal
one-sided difference is used instead, so no out-of-limit pose is ever
evaluated; the report marks such columns as `"one_sided": "forward"` or
`"backward"`.

## Input format

One JSON document (`-input`, or `-` for stdin):

```json
{
  "chain": {
    "name": "mixed-2dof",
    "joints": [
      {"name": "shoulder", "type": "revolute", "theta": 0, "d": 0,
       "a": 0.3, "alpha": 0, "offset": 0, "min": -3.14159, "max": 3.14159},
      {"name": "lift", "type": "prismatic", "theta": 0, "d": 0.2,
       "a": 0, "alpha": 0, "offset": 0, "min": 0, "max": 0.5}
    ],
    "base": [[0,-1,0,0],[1,0,0,0],[0,0,1,0.1],[0,0,0,1]],
    "tool": [[1,0,0,0.05],[0,0,-1,0],[0,1,0,0],[0,0,0,1]]
  },
  "samples": [
    {"name": "home", "positions": [0, 0.1], "velocities": [0, 0]}
  ]
}
```

`positions` and `velocities` must have one entry per joint. Samples are
processed independently: a failed sample reports `ok: false` with an error
message and no pose fields, and does not affect other samples.

## Output format

- `-json <path|->`: per-sample detail — every joint frame and the tool
  frame as world-frame 4x4 poses, the 6xN Jacobian, linear/angular tool
  velocity, and the per-column Jacobian check.
- `-csv <path|->`: one row per sample with tool position, velocity, and
  the worst Jacobian-check error; failed samples leave the value columns
  empty.

## Run

```sh
go run . -input examples/arm.json -json out.json -csv out.csv
go test ./...
```

`examples/arm.json` contains a mixed revolute+prismatic chain with a
non-identity base and a rotated tool offset, plus samples covering an
interior point, a travel-limit boundary (one-sided difference), and an
out-of-travel sample that is rejected.
