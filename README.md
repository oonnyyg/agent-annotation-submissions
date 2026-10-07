# Gated Particle Tracker

A standard-library-only Python tool that links per-frame 2D position
observations (with possible missed detections and crossing targets)
into interpretable trajectories, and reports the correspondence
between filter predictions and observations.

## Usage

```
python -m gated_particle_tracker CONFIG FRAMES [-o OUTPUT]
```

* `CONFIG`: JSON object, see below.
* `FRAMES`: JSON array of frames. Frame times must be strictly
  increasing; each frame has an `observations` array (may be empty or
  omitted) whose entries have a unique `id` (string or integer) and
  finite `x`/`y` coordinates.
* Output: JSON with per-frame track states (position, velocity,
  position covariance), per-frame association decisions, and a
  track start/end summary.

Run the tests with:

```
python -m unittest discover -s tests
```

## Configuration

| key | meaning | constraint |
| --- | --- | --- |
| `measurement_variance` | variance of each measured coordinate | > 0 |
| `acceleration_variance` | spectral density of the white acceleration noise per axis | >= 0 |
| `initial_velocity_variance` | velocity variance of a new track | >= 0 |
| `gate_threshold` | max squared Mahalanobis distance for a feasible association | >= 0 |
| `missed_penalty` | fixed cost of leaving a track unmatched in a frame | >= 0 |
| `new_track_penalty` | fixed cost of leaving an observation unmatched (spawns a track) | >= 0 |
| `max_missed` | consecutive misses after which a track ends (default 3) | integer >= 1 |
| `max_observations_per_frame` | hard limit (default 8) | integer >= 1 |
| `max_active_tracks` | hard limit (default 8) | integer >= 1 |

Exceeding either limit fails with an explicit error (exit code 2);
nothing is silently dropped.

## Model and noise conventions

* State per track: `[x, vx, y, vy]`, constant-velocity kinematics.
* Measurement: position only, `z = (x, y) + noise`, noise covariance
  `R = measurement_variance * I2`.
* Process noise: continuous white-noise acceleration model. Over each
  interval `dt` the two axes are independent and each axis is driven
  by constant white acceleration with spectral density
  `acceleration_variance`, giving the discrete per-axis noise
  covariance `q * [[dt^3/3, dt^2/2], [dt^2/2, dt]]`. Prediction uses
  the true time difference between frames.
* New tracks start with zero velocity, position variance equal to
  `measurement_variance`, and velocity variance equal to
  `initial_velocity_variance`.

## Association

For each predicted track and observation the squared Mahalanobis
distance `d^2 = r^T S^-1 r` of the innovation is computed; pairs with
`d^2 > gate_threshold` are infeasible. Among the remaining pairs the
tool solves a global one-to-one assignment (Hungarian algorithm)
minimizing the total cost: matched pairs pay `d^2`, each unmatched
track pays `missed_penalty`, each unmatched observation pays
`new_track_penalty`. Observations are never claimed greedily.

Matched tracks are updated with the standard Kalman correction
(Joseph covariance form). Unmatched observations spawn new tracks.
Unmatched tracks keep their prediction (coasting); a track ends when
its consecutive miss count reaches `max_missed`.

## Determinism and tie-breaking

Results do not depend on the order of observations within a frame:
observations are sorted by id and tracks are processed in ascending
track id before every assignment. Remaining ties (exactly equal
costs) are resolved by the Hungarian algorithm's fixed row scan order
over this canonical ordering, so repeated runs on the same input
always produce byte-identical output.

## Output format

```json
{
  "frames": [
    {
      "time": 1.0,
      "states": [
        {"track_id": 1, "position": [x, y], "velocity": [vx, vy],
         "position_covariance": [[sxx, sxy], [syx, syy]],
         "consecutive_misses": 0}
      ],
      "decisions": [
        {"type": "match", "track_id": 1, "observation_id": "a", "cost": 0.12},
        {"type": "missed", "track_id": 2, "consecutive_misses": 1},
        {"type": "new_track", "track_id": 3, "observation_id": "b"}
      ]
    }
  ],
  "tracks": [
    {"track_id": 1, "start_time": 0.0, "end_time": 3.0,
     "status": "active_at_end"}
  ]
}
```

`status` is `"ended"` when the track was terminated after
`max_missed` consecutive misses, or `"active_at_end"` when it was
still alive at the last frame.

See `examples/` for a runnable input.
