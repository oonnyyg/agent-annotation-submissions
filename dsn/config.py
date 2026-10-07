"""Configuration loading and validation for the spiking network simulator.

All quantities are unit-less but consistent: voltages (v0, reset, threshold,
stimulus levels, synaptic weights) share one unit, times (dt, tau, stimulus
bounds, delays expressed in steps) share one time unit.
"""

from __future__ import annotations

import json
import math
from dataclasses import dataclass, field
from pathlib import Path


class ConfigError(ValueError):
    """Raised when a simulation configuration is invalid."""


@dataclass(frozen=True)
class StimulusSegment:
    """Piecewise-constant equilibrium voltage over [start_step, end_step)."""

    start_step: int
    end_step: int
    v_eq: float


@dataclass(frozen=True)
class NeuronConfig:
    name: str
    tau: float
    threshold: float
    reset: float
    v0: float
    refractory_steps: int
    stimulus: tuple[StimulusSegment, ...]


@dataclass(frozen=True)
class Connection:
    source: str
    target: str
    weight: float
    delay_steps: int


@dataclass(frozen=True)
class Config:
    dt: float
    steps: int
    neurons: tuple[NeuronConfig, ...]
    connections: tuple[Connection, ...] = field(default_factory=tuple)


def _time_to_step(value: float, dt: float, what: str) -> int:
    """Convert a time to a grid step, rejecting off-grid values."""
    ratio = value / dt
    step = round(ratio)
    if not math.isclose(ratio, step, rel_tol=1e-9, abs_tol=1e-9):
        raise ConfigError(f"{what}={value} is not aligned to the time grid (dt={dt})")
    return step


def _parse_neuron(raw: dict, dt: float, steps: int, index: int) -> NeuronConfig:
    name = str(raw.get("name", f"n{index}"))
    tau = float(raw["tau"])
    threshold = float(raw["threshold"])
    reset = float(raw["reset"])
    v0 = float(raw.get("v0", reset))
    refractory_steps = int(raw.get("refractory_steps", 0))

    if tau <= 0:
        raise ConfigError(f"neuron {name}: tau must be positive")
    if not v0 < threshold:
        raise ConfigError(f"neuron {name}: v0 ({v0}) must be below threshold ({threshold})")
    if not reset < threshold:
        raise ConfigError(f"neuron {name}: reset ({reset}) must be below threshold ({threshold})")
    if refractory_steps < 0:
        raise ConfigError(f"neuron {name}: refractory_steps must be >= 0")

    segments = []
    for seg in raw.get("stimulus", []):
        start = _time_to_step(float(seg["start"]), dt, f"neuron {name} stimulus start")
        end = _time_to_step(float(seg["end"]), dt, f"neuron {name} stimulus end")
        if not start < end:
            raise ConfigError(f"neuron {name}: stimulus segment [{start}, {end}) is empty")
        segments.append(StimulusSegment(start, end, float(seg["v_eq"])))
    segments.sort(key=lambda s: s.start_step)

    if not segments or segments[0].start_step != 0 or segments[-1].end_step != steps:
        raise ConfigError(
            f"neuron {name}: stimulus must cover the whole run [0, {steps}) in steps"
        )
    for prev, nxt in zip(segments, segments[1:]):
        if prev.end_step != nxt.start_step:
            raise ConfigError(
                f"neuron {name}: stimulus segments must be contiguous and non-overlapping"
            )

    return NeuronConfig(name, tau, threshold, reset, v0, refractory_steps, tuple(segments))


def load_config(source: str | Path | dict) -> Config:
    """Load and validate a Config from a JSON path or an already-parsed dict."""
    if isinstance(source, dict):
        raw = source
    else:
        raw = json.loads(Path(source).read_text(encoding="utf-8"))

    dt = float(raw["dt"])
    steps = int(raw["steps"])
    if dt <= 0:
        raise ConfigError("dt must be positive")
    if steps <= 0:
        raise ConfigError("steps must be positive")

    neurons = tuple(
        _parse_neuron(n, dt, steps, i) for i, n in enumerate(raw.get("neurons", []))
    )
    if not neurons:
        raise ConfigError("at least one neuron is required")
    names = [n.name for n in neurons]
    if len(set(names)) != len(names):
        raise ConfigError("neuron names must be unique")
    known = set(names)

    connections = []
    for c in raw.get("connections", []):
        src, dst = str(c["from"]), str(c["to"])
        if src not in known or dst not in known:
            raise ConfigError(f"connection {src}->{dst} references an unknown neuron")
        delay = int(c["delay"])
        if delay < 1:
            raise ConfigError(f"connection {src}->{dst}: delay must be at least 1 step")
        connections.append(Connection(src, dst, float(c["weight"]), delay))

    return Config(dt, steps, neurons, tuple(connections))
