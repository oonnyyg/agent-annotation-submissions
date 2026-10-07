"""Tracking loop: predict, gate, assign, update, manage track lifecycle."""

from __future__ import annotations

from . import kalman
from .assignment import assign
from .config import ConfigError


class Track:
    __slots__ = ("track_id", "state", "misses", "start_time", "last_time")

    def __init__(self, track_id, state, time):
        self.track_id = track_id
        self.state = state
        self.misses = 0
        self.start_time = time
        self.last_time = time


def _position_covariance(p):
    return [[p[0][0], p[0][2]], [p[2][0], p[2][2]]]


class Tracker:
    def __init__(self, config):
        self.config = config
        self.tracks = []
        self._next_track_id = 1
        self.frame_results = []
        self.track_summaries = {}

    def _spawn(self, obs, time):
        track_id = self._next_track_id
        self._next_track_id += 1
        state = kalman.new_track_state(
            obs.x,
            obs.y,
            self.config.measurement_variance,
            self.config.initial_velocity_variance,
        )
        track = Track(track_id, state, time)
        self.tracks.append(track)
        self.track_summaries[track_id] = {
            "track_id": track_id,
            "start_time": time,
            "end_time": None,
            "status": "active",
        }
        return track

    def _close(self, track, status, end_time):
        summary = self.track_summaries[track.track_id]
        summary["end_time"] = end_time
        summary["status"] = status
        self.tracks.remove(track)

    def process_frame(self, frame):
        cfg = self.config
        # Deterministic ordering: observations sorted by id.
        observations = sorted(frame.observations, key=lambda o: o.obs_id)
        # Tracks are kept in ascending id order (spawn appends, ids grow).
        active = list(self.tracks)

        # Predict all active tracks to this frame's time.
        for track in active:
            dt = frame.time - track.last_time
            track.state = kalman.predict(track.state, dt, cfg.acceleration_variance)
            track.last_time = frame.time

        # Gating: squared Mahalanobis distances.
        gate_costs = []
        for track in active:
            row = []
            for obs in observations:
                residual, s = kalman.innovation(
                    track.state, obs.x, obs.y, cfg.measurement_variance
                )
                d2 = kalman.mahalanobis_squared(residual, s)
                row.append(d2 if d2 <= cfg.gate_threshold else None)
            gate_costs.append(row)

        matches, unmatched_tracks, unmatched_obs = assign(
            gate_costs, len(observations), cfg.missed_penalty, cfg.new_track_penalty
        )

        decisions = []
        for t, o, cost in matches:
            track = active[t]
            obs = observations[o]
            track.state = kalman.update(
                track.state, obs.x, obs.y, cfg.measurement_variance
            )
            track.misses = 0
            decisions.append(
                {
                    "type": "match",
                    "track_id": track.track_id,
                    "observation_id": obs.obs_id,
                    "cost": cost,
                }
            )
        for t in unmatched_tracks:
            track = active[t]
            track.misses += 1
            decisions.append(
                {
                    "type": "missed",
                    "track_id": track.track_id,
                    "consecutive_misses": track.misses,
                }
            )
        for o in unmatched_obs:
            obs = observations[o]
            track = self._spawn(obs, frame.time)
            decisions.append(
                {
                    "type": "new_track",
                    "track_id": track.track_id,
                    "observation_id": obs.obs_id,
                }
            )

        # End tracks whose consecutive miss count reached the limit.
        for track in list(self.tracks):
            if track.misses >= cfg.max_missed:
                self._close(track, "ended", frame.time)

        if len(self.tracks) > cfg.max_active_tracks:
            raise ConfigError(
                f"active track count {len(self.tracks)} exceeds the limit "
                f"of {cfg.max_active_tracks} at time {frame.time}"
            )

        states = [
            {
                "track_id": track.track_id,
                "position": [track.state.x[0], track.state.x[2]],
                "velocity": [track.state.x[1], track.state.x[3]],
                "position_covariance": _position_covariance(track.state.p),
                "consecutive_misses": track.misses,
            }
            for track in self.tracks
        ]
        decisions.sort(
            key=lambda d: (d["track_id"], d.get("observation_id", ""))
        )
        result = {
            "time": frame.time,
            "states": states,
            "decisions": decisions,
        }
        self.frame_results.append(result)
        return result

    def finalize(self, last_time):
        for track in list(self.tracks):
            self._close(track, "active_at_end", last_time)
        summaries = sorted(
            self.track_summaries.values(), key=lambda s: s["track_id"]
        )
        return {"frames": self.frame_results, "tracks": summaries}


def run_tracking(frames, config):
    tracker = Tracker(config)
    last_time = None
    for frame in frames:
        tracker.process_frame(frame)
        last_time = frame.time
    return tracker.finalize(last_time)
