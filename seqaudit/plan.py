from __future__ import annotations

import json
import math
from dataclasses import dataclass
from pathlib import Path

from .errors import PlanError

MAX_OBSERVATIONS_LIMIT = 200


@dataclass(frozen=True)
class Plan:
    p0: float
    p1: float
    lower: float
    upper: float
    max_observations: int
    true_p: float

    @property
    def step_success(self) -> float:
        return math.log(self.p1 / self.p0)

    @property
    def step_failure(self) -> float:
        return math.log((1.0 - self.p1) / (1.0 - self.p0))


def _require_number(data: dict, key: str) -> float:
    value = data.get(key)
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise PlanError(f"plan field '{key}' must be a number")
    return float(value)


def build_plan(data: dict) -> Plan:
    if not isinstance(data, dict):
        raise PlanError("plan must be a JSON object")
    p0 = _require_number(data, "p0")
    p1 = _require_number(data, "p1")
    lower = _require_number(data, "lower")
    upper = _require_number(data, "upper")
    true_p = _require_number(data, "true_p")
    max_obs = data.get("max_observations")
    if isinstance(max_obs, bool) or not isinstance(max_obs, int):
        raise PlanError("plan field 'max_observations' must be an integer")
    if not 0.0 < p0 < 1.0 or not 0.0 < p1 < 1.0:
        raise PlanError("p0 and p1 must lie strictly between 0 and 1")
    if p1 <= p0:
        raise PlanError("alternative rate p1 must exceed null rate p0")
    if not 0.0 < true_p < 1.0:
        raise PlanError("true_p must lie strictly between 0 and 1")
    if not lower < 0.0:
        raise PlanError("lower bound must be negative")
    if not upper > 0.0:
        raise PlanError("upper bound must be positive")
    if not 1 <= max_obs <= MAX_OBSERVATIONS_LIMIT:
        raise PlanError(f"max_observations must be between 1 and {MAX_OBSERVATIONS_LIMIT}")
    return Plan(p0=p0, p1=p1, lower=lower, upper=upper,
                max_observations=max_obs, true_p=true_p)


def load_plan(path: str | Path) -> Plan:
    try:
        data = json.loads(Path(path).read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise PlanError(f"plan file is not valid JSON: {exc}") from exc
    return build_plan(data)
