# gyro-attitude-replay

Replay a gyroscope record into 3D attitude using only the Python standard
library.  Angular rates are integrated as exact constant-rate rotation
increments on the quaternion manifold — never accumulated as Euler angles.

## Conventions

- **Quaternion**: Hamilton, scalar-first `(w, x, y, z)`, unit norm.  It
  represents the *active* rotation from the body frame to the world frame.
  A non-unit (but non-zero) initial quaternion is normalized; a zero
  quaternion is rejected.  `q` and `-q` describe the same attitude.
- **Frames**: all frames are right-handed.
- **Multiplication direction**: each segment's body-frame increment `dq` is
  **right-multiplied**, `q_new = q * dq`, then renormalized.  Increments
  about different axes do not commute and their order is preserved.
- **Time base**: the gyro CSV gives strictly increasing times in seconds.
  Row *i*'s angular velocity applies over the half-open interval
  `[t_i, t_{i+1})`; the last row only fixes the end of the record.  Queries
  are evaluated by integrating from the containing segment's start to the
  query time along that segment's actual rate — there is no extrapolation
  outside `[t_0, t_end]` and no shortest-arc interpolation between endpoint
  quaternions, so a single segment rotating more than half a turn is
  followed along its true path.
- **Bias**: the JSON file supplies a fixed body-frame bias (rad/s) that is
  subtracted axis-by-axis before any integration.

## Input formats

Gyro CSV (comments starting with `#` and blank lines are skipped):

```csv
# time_s, wx, wy, wz   (rad/s, body frame)
0.0, 0.0, 0.0, 1.0
0.5, 0.0, 1.0, 0.0
1.0, 0.0, 0.0, 0.0
```

Bias JSON:

```json
{"bias": [0.001, -0.002, 0.0005]}
```

Reference attitude CSV (quaternions are normalized on load):

```csv
# time_s, qw, qx, qy, qz
1.0, 0.8775825619, 0.0, 0.0, 0.4794255386
```

## Command line

```console
python -m gyro_replay gyro.csv --q0 1,0,0,0 \
    --bias-json bias.json \
    --time 0.75 --vector 1,0,0 \
    --reference-csv ref.csv --errors-out errors.json \
    --attitude-out attitude.csv
```

- `--q0 w,x,y,z` — initial attitude (required).
- `--time T` — query the attitude at an arbitrary time inside the record;
  with `--vector x,y,z` also prints the body vector rotated to world frame.
- `--attitude-out FILE` — attitude CSV (`time,qw,qx,qy,qz`) at every gyro
  row time.
- `--reference-csv FILE` — compare against reference quaternions at the
  same times; the error is the smallest attitude angle in `[0, pi]`
  (sign-invariant: `q` vs `-q` gives zero).
- `--errors-out FILE` — JSON summary with `count`, `max_rad`, `mean_rad`,
  `rms_rad` and per-sample errors (printed to stdout when no file is given).

## Library use

```python
from gyro_replay import AttitudeReplay, Quaternion, load_gyro_csv, load_bias_json

replay = AttitudeReplay(load_gyro_csv("gyro.csv"),
                        Quaternion.from_sequence([1, 0, 0, 0]),
                        bias=load_bias_json("bias.json"))
q = replay.attitude(0.75)                 # arbitrary in-record time
v_world = replay.rotate_to_world(0.75, (1, 0, 0))
err = replay.error_against(0.75, q_ref)   # radians in [0, pi]
```

## Tests

```console
python -m unittest discover -s tests
```

Covers axis-order preservation, bias subtraction, mid-segment queries of
rotations beyond half a turn, zero-rate segments, quaternion sign
equivalence, input validation, and the CLI end to end.
