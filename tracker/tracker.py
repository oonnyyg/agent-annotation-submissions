"""Multi-target tracker: gating + global assignment + track management.

Pipeline per frame:
  1. Predict every active track to the frame time (constant velocity).
  2. Gate: pairs with squared Mahalanobis distance above the
     threshold are forbidden.
  3. Solve a global one-to-one assignment minimising total cost,
     where a match costs the squared distance, an unmatched track
     costs ``miss_penalty`` and an unmatched observation costs
     ``new_penalty``.
  4. Update matched tracks, spawn tracks from unmatched
     observations, and end tracks whose consecutive miss count
     reaches ``max_misses``.

Determinism: observations are processed sorted by their id and
tracks sorted by track id, so permuting observations within a frame
does not change the result. Assignment ties are resolved in favour of
the lowest track id, then the lowest observation id.
"""

from __future__ import annotations

import math
from dataclasses import dataclass

from .assignment import hungarian_square
from .kalman import KalmanState

INF = math.inf


class TrackerError(ValueError):
    """Raised for invalid input or exceeded capacity limits."""


@dataclass(frozen=True)
class TrackerConfig:
    meas_var: float
    accel_var: float
    init_vel_var: float
    gate_threshold: float
    miss_penalty: float
    new_penalty: float
    max_misses: int
    max_tracks: int = 8
    max_observations: int = 8

    def __post_init__(self) -> None:
        def finite(name: str, value: float) -> None:
            if not math.isfinite(value):
                raise TrackerError(f"{name} must be finite, got {value!r}")

        finite("meas_var", self.meas_var)
        if self.meas_var <= 0:
            raise TrackerError("meas_var must be positive")
        for name in ("accel_var", "init_vel_var"):
            value = getattr(self, name)
            finite(name, value)
            if value < 0:
                raise TrackerError(f"{name} must be non-negative")
        finite("gate_threshold", self.gate_threshold)
        if self.gate_threshold <= 0:
            raise TrackerError("gate_threshold must be positive")
        for name in ("miss_penalty", "new_penalty"):
            value = getattr(self, name)
            finite(name, value)
            if value < 0:
                raise TrackerError(f"{name} must be non-negative")
        if self.max_misses < 1:
            raise TrackerError("max_misses must be >= 1")
        if self.max_tracks < 1 or self.max_observations < 1:
            raise TrackerError("capacity limits must be >= 1")


@dataclass
class _Track:
    track_id: int
    state: KalmanState
    start_frame: int
    last_frame: int
    misses: int = 0


def _validate_frames(frames: list[dict], config: TrackerConfig) -> None:
    prev_time = None
    for index, frame in enumerate(frames):
        time = frame.get("time")
        if not isinstance(time, (int, float)) or not math.isfinite(time):
            raise TrackerError(f"frame {index}: time must be a finite number")
        if prev_time is not None and not time > prev_time:
            raise TrackerError(f"frame {index}: times must be strictly increasing")
        prev_time = time
        observations = frame.get("observations", [])
        if len(observations) > config.max_observations:
            raise TrackerError(
                f"frame {index}: {len(observations)} observations exceeds "
                f"limit {config.max_observations}"
            )
        seen = set()
        for obs in observations:
            obs_id = obs.get("id")
            if obs_id in seen:
                raise TrackerError(f"frame {index}: duplicate observation id {obs_id!r}")
            seen.add(obs_id)
            for key in ("x", "y"):
                value = obs.get(key)
                if not isinstance(value, (int, float)) or not math.isfinite(value):
                    raise TrackerError(
                        f"frame {index}: observation {obs_id!r} has non-finite {key}"
                    )


def _associate(
    tracks: list[_Track], observations: list[dict], config: TrackerConfig
) -> tuple[list[tuple[int, int, float]], list[int], list[int]]:
    """Return (matches, unmatched_track_idx, unmatched_obs_idx).

    matches: list of (track index, observation index, cost).
    """
    n_tracks = len(tracks)
    n_obs = len(observations)
    if n_tracks == 0:
        return [], [], list(range(n_obs))
    if n_obs == 0:
        return [], list(range(n_tracks)), []

    gated = [
        [
            tracks[i].state.mahalanobis_sq(
                float(observations[j]["x"]), float(observations[j]["y"]), config.meas_var
            )
            for j in range(n_obs)
        ]
        for i in range(n_tracks)
    ]
    size = n_tracks + n_obs
    matrix = [[0.0] * size for _ in range(size)]
    for i in range(n_tracks):
        for j in range(n_obs):
            matrix[i][j] = gated[i][j] if gated[i][j] <= config.gate_threshold else INF
        for k in range(n_tracks):
            matrix[i][n_obs + k] = config.miss_penalty if k == i else INF
    for j in range(n_obs):
        for k in range(n_obs):
            matrix[n_tracks + j][k] = config.new_penalty if k == j else INF
    # Replace INF with a large finite cost for the solver.
    big = 1e18
    matrix = [[big if math.isinf(c) else c for c in row] for row in matrix]

    assignment = hungarian_square(matrix)
    matches: list[tuple[int, int, float]] = []
    unmatched_tracks: list[int] = []
    matched_obs: set[int] = set()
    for i in range(n_tracks):
        col = assignment[i]
        if col < n_obs and matrix[i][col] < big:
            matches.append((i, col, gated[i][col]))
            matched_obs.add(col)
        else:
            unmatched_tracks.append(i)
    unmatched_obs = [j for j in range(n_obs) if j not in matched_obs]
    return matches, unmatched_tracks, unmatched_obs


def track_frames(frames: list[dict], config: TrackerConfig) -> dict:
    """Run the tracker over a validated frame sequence."""
    _validate_frames(frames, config)

    tracks: list[_Track] = []
    next_track_id = 1
    ended: list[_Track] = []
    frame_outputs: list[dict] = []
    prev_time: float | None = None

    for frame_index, frame in enumerate(frames):
        time = float(frame["time"])
        observations = sorted(frame.get("observations", []), key=_obs_sort_key)
        dt = 0.0 if prev_time is None else time - prev_time
        prev_time = time

        for track in tracks:
            track.state.predict(dt, config.accel_var)

        matches, unmatched_tracks, unmatched_obs = _associate(tracks, observations, config)

        associations = []
        for track_idx, obs_idx, cost in matches:
            track = tracks[track_idx]
            obs = observations[obs_idx]
            track.state.update(float(obs["x"]), float(obs["y"]), config.meas_var)
            track.misses = 0
            track.last_frame = frame_index
            associations.append(
                {
                    "track_id": track.track_id,
                    "observation_id": obs["id"],
                    "cost": cost,
                }
            )

        new_track_ids = []
        for obs_idx in unmatched_obs:
            obs = observations[obs_idx]
            track = _Track(
                track_id=next_track_id,
                state=KalmanState.from_measurement(
                    float(obs["x"]), float(obs["y"]),
                    config.meas_var, config.init_vel_var,
                ),
                start_frame=frame_index,
                last_frame=frame_index,
            )
            next_track_id += 1
            tracks.append(track)
            new_track_ids.append(track.track_id)
            associations.append(
                {"track_id": track.track_id, "observation_id": obs["id"], "cost": None}
            )

        if len(tracks) > config.max_tracks:
            raise TrackerError(
                f"frame {frame_index}: {len(tracks)} active tracks exceeds "
                f"limit {config.max_tracks}"
            )

        for track_idx in unmatched_tracks:
            tracks[track_idx].misses += 1

        ended_track_ids = []
        still_active = []
        for track in tracks:
            if track.misses >= config.max_misses:
                ended.append(track)
                ended_track_ids.append(track.track_id)
            else:
                still_active.append(track)
        tracks = still_active

        frame_outputs.append(
            {
                "time": time,
                "tracks": [
                    {
                        "track_id": t.track_id,
                        "x": t.state.position[0],
                        "y": t.state.position[1],
                        "covariance": t.state.position_covariance(),
                        "misses": t.misses,
                    }
                    for t in sorted(tracks, key=lambda t: t.track_id)
                ],
                "associations": sorted(associations, key=lambda a: a["track_id"]),
                "new_track_ids": sorted(new_track_ids),
                "ended_track_ids": sorted(ended_track_ids),
            }
        )

    ended.extend(tracks)
    summary = [
        {
            "track_id": t.track_id,
            "start_frame": t.start_frame,
            "end_frame": t.last_frame,
            "length": t.last_frame - t.start_frame + 1,
        }
        for t in sorted(ended, key=lambda t: t.track_id)
    ]
    return {"frames": frame_outputs, "summary": summary}


def _obs_sort_key(obs: dict) -> tuple:
    """Deterministic observation ordering: numbers before strings."""
    obs_id = obs["id"]
    if isinstance(obs_id, bool) or not isinstance(obs_id, (int, float, str)):
        raise TrackerError(f"observation id must be a number or string, got {obs_id!r}")
    return (0, obs_id, "") if isinstance(obs_id, (int, float)) else (1, 0, obs_id)
