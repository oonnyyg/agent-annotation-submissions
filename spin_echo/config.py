"""Configuration loading and validation for the spin echo simulator."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass, field


class ConfigError(ValueError):
    """Raised when the configuration is invalid, with a located message."""


def _number(value, path, allow_inf=False):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{path}: expected a number, got {value!r}")
    v = float(value)
    if math.isnan(v):
        raise ConfigError(f"{path}: NaN is not allowed")
    if math.isinf(v) and not allow_inf:
        raise ConfigError(f"{path}: infinite value is not allowed here")
    return v


def _relaxation(value, path):
    """Relaxation time in seconds; must be > 0, may be infinite."""
    if value is None:
        return math.inf
    v = _number(value, path, allow_inf=True)
    if v <= 0.0:
        raise ConfigError(f"{path}: relaxation time must be positive, got {v}")
    return v


@dataclass
class SpinGroup:
    name: str
    weight: float
    m0: tuple  # (Mx, My, Mz) initial magnetization
    offset_hz: float
    t1: float  # seconds, may be inf
    t2: float  # seconds, may be inf
    mz_eq: float  # equilibrium longitudinal magnetization


@dataclass
class Pulse:
    time: float  # seconds
    phase: float  # radians, axis in transverse plane
    flip: float  # radians, right-hand rotation about the phase axis
    order: int  # input order, for stable sequencing at equal times


@dataclass
class Config:
    spins: list
    pulses: list
    samples: list  # sorted list of unique sample times
    window: tuple  # (start, end) peak-report window, may contain inf


def _parse_spin(raw, path):
    if not isinstance(raw, dict):
        raise ConfigError(f"{path}: expected an object")
    name = raw.get("name")
    if name is None:
        name = path
    elif not isinstance(name, str):
        raise ConfigError(f"{path}.name: expected a string")
    weight = _number(raw.get("weight"), f"{path}.weight")
    if weight < 0.0:
        raise ConfigError(f"{path}.weight: must be non-negative, got {weight}")
    m0raw = raw.get("m0")
    if not isinstance(m0raw, (list, tuple)) or len(m0raw) != 3:
        raise ConfigError(f"{path}.m0: expected a 3-element array [Mx, My, Mz]")
    m0 = tuple(_number(c, f"{path}.m0[{i}]") for i, c in enumerate(m0raw))
    offset = _number(raw.get("offset_hz"), f"{path}.offset_hz")
    t1 = _relaxation(raw.get("t1"), f"{path}.t1")
    t2 = _relaxation(raw.get("t2"), f"{path}.t2")
    mz_eq = _number(raw.get("mz_eq", 0.0), f"{path}.mz_eq")
    return SpinGroup(name, weight, m0, offset, t1, t2, mz_eq)


def _parse_pulse(raw, path, order):
    if not isinstance(raw, dict):
        raise ConfigError(f"{path}: expected an object")
    time = _number(raw.get("time"), f"{path}.time")
    if time < 0.0:
        raise ConfigError(f"{path}.time: must be non-negative, got {time}")
    phase = _number(raw.get("phase", 0.0), f"{path}.phase")
    flip = _number(raw.get("flip"), f"{path}.flip")
    return Pulse(time, phase, flip, order)


def load_config(text):
    """Parse and validate a JSON configuration string."""
    try:
        raw = json.loads(text)
    except json.JSONDecodeError as exc:
        raise ConfigError(f"config: invalid JSON at line {exc.lineno}: {exc.msg}") from exc
    if not isinstance(raw, dict):
        raise ConfigError("config: top level must be an object")

    spins_raw = raw.get("spins")
    if not isinstance(spins_raw, list) or not spins_raw:
        raise ConfigError("config.spins: expected a non-empty array")
    spins = [_parse_spin(s, f"config.spins[{i}]") for i, s in enumerate(spins_raw)]
    if sum(s.weight for s in spins) <= 0.0:
        raise ConfigError("config.spins: total weight must be positive")

    pulses_raw = raw.get("pulses", [])
    if not isinstance(pulses_raw, list):
        raise ConfigError("config.pulses: expected an array")
    pulses = [
        _parse_pulse(p, f"config.pulses[{i}]", i)
        for i, p in enumerate(pulses_raw)
    ]

    samples_raw = raw.get("samples")
    if not isinstance(samples_raw, list) or not samples_raw:
        raise ConfigError("config.samples: expected a non-empty array")
    samples = []
    for i, s in enumerate(samples_raw):
        t = _number(s, f"config.samples[{i}]")
        if t < 0.0:
            raise ConfigError(f"config.samples[{i}]: must be non-negative, got {t}")
        samples.append(t)
    samples = sorted(set(samples))

    window_raw = raw.get("window")
    if window_raw is None:
        window = (0.0, math.inf)
    else:
        if not isinstance(window_raw, list) or len(window_raw) != 2:
            raise ConfigError("config.window: expected [start, end]")
        start = _number(window_raw[0], "config.window[0]", allow_inf=True)
        end = _number(window_raw[1], "config.window[1]", allow_inf=True)
        if not end >= start:
            raise ConfigError("config.window: end must be >= start")
        window = (start, end)

    return Config(spins=spins, pulses=pulses, samples=samples, window=window)
