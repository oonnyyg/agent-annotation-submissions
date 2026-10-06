# gyro-attitude-replay

Replay a body-rate gyroscope log into 3D attitude using only the Python
standard library. Angular rates are integrated as exact constant-rate
rotation increments on the unit quaternion group — the three axis rates
are never accumulated as Euler angles.

## Conventions

- Quaternions are Hamilton, scalar-first `(w, x, y, z)`, representing the
  active rotation from the body frame to the world frame. All frames are
  right-handed.
- Multiplication direction: each segment's increment `dq` is
  right-multiplied onto the current attitude, `q_new = q * dq`, where
  `dq = (cos(|w|dt/2), w/|w| * sin(|w|dt/2))` for the bias-corrected body
  rate `w`. Order is preserved; increments about different axes are never
  commuted.
- Time convention: CSV row `i` gives the rate that holds over
  `[t_i, t_{i+1})`; the final row only fixes the end time. Queries are
  answered anywhere inside `[t_0, t_end]` by integrating the containing
  segment's exact increment up to `t` — never by extrapolation, and never
  by shortest-arc interpolation between knot quaternions (so a single
  segment rotating more than half a turn evolves along the actual rate
  vector).
- Attitude error to a reference is `2*acos(|dot|)` in `[0, pi]`, so `q`
  and `-q` (the same attitude) give zero error.

## Input formats

- Gyro CSV: `t,wx,wy,wz` (optional header), `t` in seconds strictly
  increasing, rates in rad/s, at least two rows, all values finite.
- Bias JSON: `{"wx": ..., "wy": ..., "wz": ...}` or `[wx, wy, wz]`,
  subtracted per axis before integration.
- Reference CSV: `t,qw,qx,qy,qz` for error comparison.

## Usage

```
python -m gyro_replay.cli gyro.csv --bias-json bias.json \
    --q0 1,0,0,0 --out attitude.csv \
    --reference-csv ref.csv --error-json errors.json \
    --vector 1,0,0
```

Outputs an attitude CSV (`t,qw,qx,qy,qz`, plus rotated-vector columns if
`--vector` is given) and a JSON summary with count, max/mean/RMS attitude
error in radians and per-sample errors.

## Library

```python
from gyro_replay import GyroReplay, Quaternion, load_gyro_csv, load_bias_json

times, rates = load_gyro_csv("gyro.csv")
replay = GyroReplay(times, rates, bias=load_bias_json("bias.json"))
q = replay.attitude(1.25)            # any t within the log
v_world = replay.rotate_to_world(1.25, (1, 0, 0))
err = q.attitude_error(q_reference)  # 0..pi
```

## Tests

```
python -m unittest discover -s tests
```
