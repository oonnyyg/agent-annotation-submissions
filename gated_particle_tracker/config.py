"""Configuration and input validation for the tracker."""

from __future__ import annotations

import math
from dataclasses import dataclass


class ConfigError(ValueError):
    """Raised when configuration or input data is invalid."""


def _finite_number(value, name):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{name} must be a number, got {value!r}")
    if not math.isfinite(value):
        raise ConfigError(f"{name} must be finite, got {value!r}")
    return float(value)


def _nonnegative(value, name):
    value = _finite_number(value, name)
    if value < 0.0:
        raise ConfigError(f"{name} must be non-negative, got {value!r}")
    return value


@dataclass(frozen=True)
class Config:
    """Tracker configuration.

    Attributes:
        measurement_variance: variance of each position coordinate of a
            measurement. Must be strictly positive.
        acceleration_variance: variance of the per-axis constant
            acceleration noise acting over each time interval
            (continuous white-noise acceleration model). Non-negative.
        initial_velocity_variance: variance of each velocity component
            for a newly created track. Non-negative.
        gate_threshold: maximum allowed squared Mahalanobis distance
            for a feasible track-observation association. Non-negative.
        missed_penalty: fixed cost added for each unmatched track.
        new_track_penalty: fixed cost added for each unmatched
            observation (i.e. for spawning a new track).
        max_missed: number of consecutive missed frames after which a
            track ends. Must be a positive integer; a track ends when
            its consecutive miss count reaches this value.
        max_observations_per_frame: hard limit, default 8.
        max_active_tracks: hard limit, default 8.
    """

    measurement_variance: float
    acceleration_variance: float
    initial_velocity_variance: float
    gate_threshold: float
    missed_penalty: float
    new_track_penalty: float
    max_missed: int = 3
    max_observations_per_frame: int = 8
    max_active_tracks: int = 8

    @classmethod
    def from_dict(cls, data):
        if not isinstance(data, dict):
            raise ConfigError("config must be a JSON object")
        required = (
            "measurement_variance",
            "acceleration_variance",
            "initial_velocity_variance",
            "gate_threshold",
            "missed_penalty",
            "new_track_penalty",
        )
        missing = [k for k in required if k not in data]
        if missing:
            raise ConfigError(f"config missing keys: {', '.join(missing)}")

        measurement_variance = _finite_number(
            data["measurement_variance"], "measurement_variance"
        )
        if measurement_variance <= 0.0:
            raise ConfigError("measurement_variance must be positive")

        max_missed = data.get("max_missed", 3)
        if isinstance(max_missed, bool) or not isinstance(max_missed, int):
            raise ConfigError("max_missed must be an integer")
        if max_missed < 1:
            raise ConfigError("max_missed must be >= 1")

        limits = {}
        for key in ("max_observations_per_frame", "max_active_tracks"):
            value = data.get(key, 8)
            if isinstance(value, bool) or not isinstance(value, int) or value < 1:
                raise ConfigError(f"{key} must be a positive integer")
            limits[key] = value

        return cls(
            measurement_variance=measurement_variance,
            acceleration_variance=_nonnegative(
                data["acceleration_variance"], "acceleration_variance"
            ),
            initial_velocity_variance=_nonnegative(
                data["initial_velocity_variance"], "initial_velocity_variance"
            ),
            gate_threshold=_nonnegative(data["gate_threshold"], "gate_threshold"),
            missed_penalty=_nonnegative(data["missed_penalty"], "missed_penalty"),
            new_track_penalty=_nonnegative(
                data["new_track_penalty"], "new_track_penalty"
            ),
            max_missed=max_missed,
            **limits,
        )
