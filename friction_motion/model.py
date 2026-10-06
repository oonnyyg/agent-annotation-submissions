"""Input schema and validation for friction-motion scenarios.

All quantities are SI: metres, seconds, kilograms, newtons, m/s^2.
"""

from __future__ import annotations

import json
from dataclasses import dataclass
from pathlib import Path


class ScenarioError(ValueError):
    """Raised when a scenario definition is invalid."""


@dataclass(frozen=True)
class ForceSegment:
    """Constant external force applied on [start, end); end is the next
    segment's start, or t_end for the last segment."""

    start: float
    force: float


@dataclass(frozen=True)
class Scenario:
    name: str
    mass: float
    gravity: float
    mu_static: float
    mu_kinetic: float
    x0: float
    v0: float
    t_end: float
    sample_dt: float
    segments: tuple[ForceSegment, ...]

    @property
    def normal_force(self) -> float:
        return self.mass * self.gravity

    @property
    def max_static_friction(self) -> float:
        return self.mu_static * self.normal_force

    @property
    def kinetic_friction(self) -> float:
        return self.mu_kinetic * self.normal_force


def _require_number(data: dict, key: str, ctx: str) -> float:
    value = data.get(key)
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ScenarioError(f"{ctx}: '{key}' must be a number, got {value!r}")
    return float(value)


def parse_scenario(data: dict, index: int) -> Scenario:
    ctx = f"scenario[{index}]"
    if not isinstance(data, dict):
        raise ScenarioError(f"{ctx}: must be an object")
    name = data.get("name", f"scenario_{index}")
    if not isinstance(name, str) or not name:
        raise ScenarioError(f"{ctx}: 'name' must be a non-empty string")
    ctx = f"scenario '{name}'"

    mass = _require_number(data, "mass", ctx)
    gravity = _require_number(data, "gravity", ctx)
    mu_s = _require_number(data, "mu_static", ctx)
    mu_k = _require_number(data, "mu_kinetic", ctx)
    x0 = float(data.get("x0", 0.0))
    v0 = float(data.get("v0", 0.0))
    t_end = _require_number(data, "t_end", ctx)
    sample_dt = _require_number(data, "sample_dt", ctx)

    if mass <= 0:
        raise ScenarioError(f"{ctx}: mass must be positive")
    if gravity <= 0:
        raise ScenarioError(f"{ctx}: gravity must be positive")
    if mu_s < 0 or mu_k < 0:
        raise ScenarioError(f"{ctx}: friction coefficients must be non-negative")
    if mu_s < mu_k:
        raise ScenarioError(
            f"{ctx}: mu_static ({mu_s}) must be >= mu_kinetic ({mu_k})"
        )
    if t_end <= 0:
        raise ScenarioError(f"{ctx}: t_end must be positive")
    if sample_dt <= 0:
        raise ScenarioError(f"{ctx}: sample_dt must be positive")

    raw_segments = data.get("force_segments")
    if not isinstance(raw_segments, list) or not raw_segments:
        raise ScenarioError(f"{ctx}: 'force_segments' must be a non-empty list")
    segments = []
    for i, entry in enumerate(raw_segments):
        sctx = f"{ctx} force_segments[{i}]"
        if isinstance(entry, dict):
            start = _require_number(entry, "start", sctx)
            force = _require_number(entry, "force", sctx)
        elif (
            isinstance(entry, (list, tuple))
            and len(entry) == 2
            and all(isinstance(v, (int, float)) for v in entry)
        ):
            start, force = float(entry[0]), float(entry[1])
        else:
            raise ScenarioError(
                f"{sctx}: must be {{'start', 'force'}} or [start, force]"
            )
        segments.append(ForceSegment(start, force))

    segments.sort(key=lambda s: s.start)
    if abs(segments[0].start) > 1e-12:
        raise ScenarioError(f"{ctx}: force segments must start at t=0")
    for prev, cur in zip(segments, segments[1:]):
        if cur.start <= prev.start:
            raise ScenarioError(f"{ctx}: force segment starts must increase")
    if segments[-1].start >= t_end:
        raise ScenarioError(f"{ctx}: force segments must cover [0, t_end]")

    return Scenario(
        name=name,
        mass=mass,
        gravity=gravity,
        mu_static=mu_s,
        mu_kinetic=mu_k,
        x0=x0,
        v0=v0,
        t_end=t_end,
        sample_dt=sample_dt,
        segments=tuple(segments),
    )


def load_scenarios(path: str | Path) -> list[Scenario]:
    """Load one scenario object or a list / {'scenarios': [...]} from JSON."""
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    if isinstance(data, dict) and "scenarios" in data:
        items = data["scenarios"]
    elif isinstance(data, list):
        items = data
    else:
        items = [data]
    if not isinstance(items, list) or not items:
        raise ScenarioError("input JSON must contain at least one scenario")
    return [parse_scenario(item, i) for i, item in enumerate(items)]
