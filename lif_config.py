"""JSON configuration loading and validation for the LIF simulator.

Configuration schema (all voltages in mV, times in ms)::

    {
      "dt": 0.5,
      "duration": 20.0,
      "neurons": [
        {
          "name": "A",
          "tau": 10.0,
          "threshold": -50.0,
          "reset": -65.0,
          "initial": -70.0,
          "refractory_steps": 2
        }
      ],
      "synapses": [
        {"source": "A", "target": "A", "weight": 5.0, "delay_steps": 1}
      ],
      "stimulus": {
        "A": [[0.0, 10.0, -55.0], [10.0, 20.0, -70.0]]
      }
    }

``stimulus`` maps a neuron name to a list of ``[start_ms, end_ms,
equilibrium_mV]`` segments. Segments must align to the time grid and
together cover ``[0, duration)`` exactly, without gaps or overlaps.
"""

from __future__ import annotations

import json
import math
from pathlib import Path

from lif_sim import Network, NeuronConfig, SynapseConfig


class ConfigError(ValueError):
    """Raised when a JSON configuration is invalid."""


def _require_number(value: object, what: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{what} must be a number, got {value!r}")
    result = float(value)
    if not math.isfinite(result):
        raise ConfigError(f"{what} must be finite, got {value!r}")
    return result


def _require_int(value: object, what: str) -> int:
    if isinstance(value, bool) or not isinstance(value, int):
        raise ConfigError(f"{what} must be an integer, got {value!r}")
    return value


def _step_index(time_ms: float, dt: float, what: str) -> int:
    """Convert a grid-aligned time in ms to an integer step index."""
    ratio = time_ms / dt
    rounded = round(ratio)
    if abs(ratio - rounded) > 1e-9:
        raise ConfigError(
            f"{what} = {time_ms} ms is not aligned to the time grid "
            f"(dt = {dt} ms)"
        )
    return rounded


def load_config(path: str | Path) -> Network:
    """Load, validate and build a :class:`Network` from a JSON file."""
    with open(path, "r", encoding="utf-8") as handle:
        raw = json.load(handle)
    return build_network(raw)


def build_network(raw: dict) -> Network:
    """Validate a parsed JSON dict and build a :class:`Network`."""
    if not isinstance(raw, dict):
        raise ConfigError("top-level configuration must be an object")

    dt = _require_number(raw.get("dt"), "dt")
    if dt <= 0:
        raise ConfigError("dt must be positive")
    duration = _require_number(raw.get("duration"), "duration")
    num_steps = _step_index(duration, dt, "duration")
    if num_steps < 1:
        raise ConfigError("duration must cover at least one time step")

    raw_neurons = raw.get("neurons")
    if not isinstance(raw_neurons, list) or not raw_neurons:
        raise ConfigError("neurons must be a non-empty list")

    neurons: list[NeuronConfig] = []
    names: list[str] = []
    for index, entry in enumerate(raw_neurons):
        if not isinstance(entry, dict):
            raise ConfigError(f"neurons[{index}] must be an object")
        name = entry.get("name", f"n{index}")
        if not isinstance(name, str) or not name:
            raise ConfigError(f"neurons[{index}].name must be a string")
        if name in names:
            raise ConfigError(f"duplicate neuron name {name!r}")
        tau = _require_number(entry.get("tau"), f"{name}.tau")
        if tau <= 0:
            raise ConfigError(f"{name}.tau must be positive")
        threshold = _require_number(entry.get("threshold"), f"{name}.threshold")
        reset = _require_number(entry.get("reset"), f"{name}.reset")
        initial = _require_number(entry.get("initial"), f"{name}.initial")
        refractory = _require_int(
            entry.get("refractory_steps", 0), f"{name}.refractory_steps"
        )
        if refractory < 0:
            raise ConfigError(f"{name}.refractory_steps must be >= 0")
        if reset >= threshold:
            raise ConfigError(
                f"{name}: reset ({reset}) must be below threshold ({threshold})"
            )
        if initial >= threshold:
            raise ConfigError(
                f"{name}: initial ({initial}) must be below threshold "
                f"({threshold})"
            )
        neurons.append(
            NeuronConfig(
                tau=tau,
                threshold=threshold,
                reset=reset,
                initial=initial,
                refractory_steps=refractory,
            )
        )
        names.append(name)

    index_of = {name: i for i, name in enumerate(names)}

    raw_synapses = raw.get("synapses", [])
    if not isinstance(raw_synapses, list):
        raise ConfigError("synapses must be a list")
    synapses: list[SynapseConfig] = []
    for index, entry in enumerate(raw_synapses):
        if not isinstance(entry, dict):
            raise ConfigError(f"synapses[{index}] must be an object")
        source = entry.get("source")
        target = entry.get("target")
        if source not in index_of:
            raise ConfigError(f"synapses[{index}]: unknown source {source!r}")
        if target not in index_of:
            raise ConfigError(f"synapses[{index}]: unknown target {target!r}")
        weight = _require_number(entry.get("weight"), f"synapses[{index}].weight")
        delay = _require_int(
            entry.get("delay_steps"), f"synapses[{index}].delay_steps"
        )
        if delay < 1:
            raise ConfigError(
                f"synapses[{index}].delay_steps must be at least 1"
            )
        synapses.append(
            SynapseConfig(
                source=index_of[source],
                target=index_of[target],
                weight=weight,
                delay_steps=delay,
            )
        )

    raw_stimulus = raw.get("stimulus")
    if not isinstance(raw_stimulus, dict):
        raise ConfigError("stimulus must be an object mapping names to segments")
    unknown = set(raw_stimulus) - set(names)
    if unknown:
        raise ConfigError(f"stimulus references unknown neurons: {sorted(unknown)}")
    missing = [name for name in names if name not in raw_stimulus]
    if missing:
        raise ConfigError(f"stimulus missing for neurons: {missing}")

    equilibrium = [[0.0] * len(neurons) for _ in range(num_steps)]
    for name in names:
        segments = raw_stimulus[name]
        if not isinstance(segments, list) or not segments:
            raise ConfigError(f"stimulus[{name!r}] must be a non-empty list")
        covered = [False] * num_steps
        for seg_index, segment in enumerate(segments):
            what = f"stimulus[{name!r}][{seg_index}]"
            if not isinstance(segment, (list, tuple)) or len(segment) != 3:
                raise ConfigError(f"{what} must be [start_ms, end_ms, mV]")
            start = _require_number(segment[0], f"{what} start")
            end = _require_number(segment[1], f"{what} end")
            value = _require_number(segment[2], f"{what} equilibrium")
            start_step = _step_index(start, dt, f"{what} start")
            end_step = _step_index(end, dt, f"{what} end")
            if not (0 <= start_step < end_step <= num_steps):
                raise ConfigError(
                    f"{what}: need 0 <= start < end <= duration "
                    f"({duration} ms)"
                )
            column = index_of[name]
            for step in range(start_step, end_step):
                if covered[step]:
                    raise ConfigError(
                        f"{what}: overlapping stimulus segments for {name!r}"
                    )
                covered[step] = True
                equilibrium[step][column] = value
        if not all(covered):
            raise ConfigError(
                f"stimulus for {name!r} does not cover the whole "
                f"simulation period [0, {duration}) ms"
            )

    return Network(
        dt=dt,
        num_steps=num_steps,
        neurons=neurons,
        neuron_names=names,
        synapses=synapses,
        equilibrium=equilibrium,
    )
