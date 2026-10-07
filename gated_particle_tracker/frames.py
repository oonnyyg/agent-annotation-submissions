"""Frame sequence loading and validation."""

from __future__ import annotations

from dataclasses import dataclass

from .config import ConfigError, _finite_number


@dataclass(frozen=True)
class Observation:
    obs_id: str
    x: float
    y: float


@dataclass(frozen=True)
class Frame:
    time: float
    observations: tuple


def parse_frames(data, max_observations_per_frame):
    """Validate a decoded JSON frame list and return Frame objects."""
    if not isinstance(data, list):
        raise ConfigError("frames must be a JSON array")
    frames = []
    previous_time = None
    for index, raw in enumerate(data):
        if not isinstance(raw, dict):
            raise ConfigError(f"frame {index} must be an object")
        if "time" not in raw:
            raise ConfigError(f"frame {index} missing 'time'")
        time = _finite_number(raw["time"], f"frame {index} time")
        if previous_time is not None and time <= previous_time:
            raise ConfigError(
                f"frame {index} time {time} is not strictly greater than "
                f"previous time {previous_time}"
            )
        previous_time = time

        raw_obs = raw.get("observations", [])
        if not isinstance(raw_obs, list):
            raise ConfigError(f"frame {index} observations must be an array")
        if len(raw_obs) > max_observations_per_frame:
            raise ConfigError(
                f"frame {index} has {len(raw_obs)} observations, exceeding "
                f"the limit of {max_observations_per_frame}"
            )
        observations = []
        seen_ids = set()
        for obs_index, obs in enumerate(raw_obs):
            where = f"frame {index} observation {obs_index}"
            if not isinstance(obs, dict):
                raise ConfigError(f"{where} must be an object")
            obs_id = obs.get("id")
            if isinstance(obs_id, bool) or not isinstance(obs_id, (str, int)):
                raise ConfigError(f"{where} id must be a string or integer")
            obs_id = str(obs_id)
            if obs_id in seen_ids:
                raise ConfigError(
                    f"frame {index} has duplicate observation id {obs_id!r}"
                )
            seen_ids.add(obs_id)
            x = _finite_number(obs.get("x"), f"{where} x")
            y = _finite_number(obs.get("y"), f"{where} y")
            observations.append(Observation(obs_id, x, y))
        frames.append(Frame(time, tuple(observations)))
    return frames
