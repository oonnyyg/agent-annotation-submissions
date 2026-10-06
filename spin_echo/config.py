"""Parsing and validation of the JSON experiment configuration."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .errors import ConfigError

_INF_STRINGS = {"inf", "+inf", "infinity", "+infinity"}


def _number(value, loc: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{loc}: expected a number, got {value!r}")
    result = float(value)
    if math.isnan(result):
        raise ConfigError(f"{loc}: NaN is not allowed")
    return result


def _finite(value, loc: str) -> float:
    result = _number(value, loc)
    if math.isinf(result):
        raise ConfigError(f"{loc}: must be finite, got {result}")
    return result


def _relaxation_time(value, loc: str) -> float:
    """Relaxation times are positive seconds; infinity is allowed."""
    if isinstance(value, str) and value.strip().lower() in _INF_STRINGS:
        return math.inf
    result = _number(value, loc)
    if result <= 0.0:
        raise ConfigError(f"{loc}: relaxation time must be positive, got {result}")
    return result


@dataclass
class SpinGroup:
    weight: float
    initial: tuple[float, float, float]
    offset_hz: float
    t1: float
    t2: float
    equilibrium_mz: float


@dataclass
class Pulse:
    time: float
    phase_rad: float
    flip_rad: float


@dataclass
class Experiment:
    spins: list[SpinGroup]
    pulses: list[Pulse]
    sample_times: list[float]
    peak_window: tuple[float, float] = (-math.inf, math.inf)
    extra: dict = field(default_factory=dict)


def _parse_spin(raw, index: int) -> SpinGroup:
    loc = f"spins[{index}]"
    if not isinstance(raw, dict):
        raise ConfigError(f"{loc}: expected an object")
    try:
        weight = _number(raw["weight"], f"{loc}.weight")
    except KeyError:
        raise ConfigError(f"{loc}.weight: missing required field") from None
    if weight < 0.0:
        raise ConfigError(f"{loc}.weight: weight must be non-negative, got {weight}")

    initial = raw.get("initial", [0.0, 1.0, 0.0])
    if (
        not isinstance(initial, (list, tuple))
        or len(initial) != 3
    ):
        raise ConfigError(f"{loc}.initial: expected a 3-element array [mx, my, mz]")
    initial = tuple(_finite(v, f"{loc}.initial[{i}]") for i, v in enumerate(initial))

    offset = _finite(raw.get("offset_hz", 0.0), f"{loc}.offset_hz")
    t1 = _relaxation_time(raw.get("t1", "inf"), f"{loc}.t1")
    t2 = _relaxation_time(raw.get("t2", "inf"), f"{loc}.t2")
    meq = _finite(raw.get("equilibrium_mz", 1.0), f"{loc}.equilibrium_mz")
    return SpinGroup(weight, initial, offset, t1, t2, meq)


def _parse_pulse(raw, index: int) -> Pulse:
    loc = f"pulses[{index}]"
    if not isinstance(raw, dict):
        raise ConfigError(f"{loc}: expected an object")
    if "time" not in raw:
        raise ConfigError(f"{loc}.time: missing required field")
    time = _finite(raw["time"], f"{loc}.time")
    if time < 0.0:
        raise ConfigError(f"{loc}.time: must be non-negative, got {time}")
    phase = math.radians(_finite(raw.get("phase_deg", 0.0), f"{loc}.phase_deg"))
    flip = math.radians(_finite(raw.get("flip_deg", 180.0), f"{loc}.flip_deg"))
    return Pulse(time, phase, flip)


def _parse_sample_times(raw) -> list[float]:
    if raw is None:
        raise ConfigError("samples: missing required section")
    if isinstance(raw, list):
        times = [_finite(v, f"samples[{i}]") for i, v in enumerate(raw)]
    elif isinstance(raw, dict):
        if "times" in raw:
            times = [
                _finite(v, f"samples.times[{i}]")
                for i, v in enumerate(raw["times"])
            ]
        else:
            for key in ("start", "stop", "step"):
                if key not in raw:
                    raise ConfigError(f"samples.{key}: missing required field")
            start = _finite(raw["start"], "samples.start")
            stop = _finite(raw["stop"], "samples.stop")
            step = _number(raw["step"], "samples.step")
            if step <= 0.0:
                raise ConfigError(f"samples.step: must be positive, got {step}")
            if stop < start:
                raise ConfigError(
                    f"samples.stop: must be >= samples.start ({stop} < {start})"
                )
            count = int(round((stop - start) / step))
            times = [start + k * step for k in range(count + 1)]
    else:
        raise ConfigError("samples: expected an object or an array of times")
    for i, t in enumerate(times):
        if t < 0.0:
            raise ConfigError(f"sample time at index {i}: must be non-negative, got {t}")
    return sorted(set(times))


def load_experiment(data) -> Experiment:
    if not isinstance(data, dict):
        raise ConfigError("root: expected a JSON object")
    raw_spins = data.get("spins")
    if not isinstance(raw_spins, list) or not raw_spins:
        raise ConfigError("spins: expected a non-empty array")
    spins = [_parse_spin(s, i) for i, s in enumerate(raw_spins)]
    if sum(s.weight for s in spins) <= 0.0:
        raise ConfigError("spins: total weight must be positive")

    raw_pulses = data.get("pulses", [])
    if not isinstance(raw_pulses, list):
        raise ConfigError("pulses: expected an array")
    pulses = [_parse_pulse(p, i) for i, p in enumerate(raw_pulses)]

    sample_times = _parse_sample_times(data.get("samples"))

    window = data.get("peak_window")
    if window is None:
        peak_window = (-math.inf, math.inf)
    else:
        if not isinstance(window, (list, tuple)) or len(window) != 2:
            raise ConfigError("peak_window: expected [t_start, t_end]")
        lo = _finite(window[0], "peak_window[0]")
        hi = _finite(window[1], "peak_window[1]")
        if hi < lo:
            raise ConfigError(f"peak_window: end {hi} is before start {lo}")
        peak_window = (lo, hi)

    return Experiment(spins, pulses, sample_times, peak_window)
