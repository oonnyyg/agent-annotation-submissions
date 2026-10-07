"""Plan loading and strict input validation."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass
from pathlib import Path

MAX_OBSERVATIONS_LIMIT = 200


class InputError(ValueError):
    """Raised when a plan or observation file is invalid; the whole run is rejected."""


@dataclass(frozen=True)
class Plan:
    plan_id: str
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


@dataclass(frozen=True)
class Observation:
    series_id: str
    position: int
    outcome: int


def _require(cond: bool, message: str) -> None:
    if not cond:
        raise InputError(message)


def load_plan(path: str | Path) -> Plan:
    try:
        raw = json.loads(Path(path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise InputError(f"cannot read plan file {path}: {exc}") from exc
    _require(isinstance(raw, dict), "plan JSON must be an object")

    def field(name: str):
        _require(name in raw, f"plan missing field '{name}'")
        return raw[name]

    plan_id = field("plan_id")
    _require(isinstance(plan_id, str) and plan_id.strip(), "plan_id must be a non-empty string")

    p0, p1, true_p = field("p0"), field("p1"), field("true_p")
    for name, value in (("p0", p0), ("p1", p1), ("true_p", true_p)):
        _require(
            isinstance(value, (int, float)) and not isinstance(value, bool) and 0.0 < value < 1.0,
            f"{name} must lie strictly between 0 and 1",
        )
    _require(p1 > p0, "alternative success rate p1 must exceed null rate p0")

    lower, upper = field("lower"), field("upper")
    _require(isinstance(lower, (int, float)) and lower < 0, "lower bound must be negative")
    _require(isinstance(upper, (int, float)) and upper > 0, "upper bound must be positive")

    max_obs = field("max_observations")
    _require(
        isinstance(max_obs, int) and not isinstance(max_obs, bool) and 1 <= max_obs <= MAX_OBSERVATIONS_LIMIT,
        f"max_observations must be an integer in [1, {MAX_OBSERVATIONS_LIMIT}]",
    )

    return Plan(plan_id, float(p0), float(p1), float(lower), float(upper), max_obs, float(true_p))


def load_observations(path: str | Path) -> dict[str, list[Observation]]:
    try:
        with open(path, newline="", encoding="utf-8") as handle:
            rows = list(csv.DictReader(handle))
    except OSError as exc:
        raise InputError(f"cannot read observations file {path}: {exc}") from exc
    _require(rows, "observations CSV is empty")
    _require(
        rows[0] is not None and {"series_id", "position", "outcome"} <= set(rows[0].keys() or {}),
        "observations CSV must have columns series_id,position,outcome",
    )

    series: dict[str, list[Observation]] = {}
    seen: dict[str, set[int]] = {}
    for line, row in enumerate(rows, start=2):
        sid = (row.get("series_id") or "").strip()
        _require(sid, f"line {line}: series_id must be non-empty")
        try:
            position = int(row["position"])
        except (TypeError, ValueError):
            raise InputError(f"line {line}: position must be an integer") from None
        _require(position >= 1, f"line {line}: position must be >= 1")
        outcome_raw = (row.get("outcome") or "").strip()
        _require(outcome_raw in ("0", "1"), f"line {line}: outcome must be 0 or 1")
        bucket = seen.setdefault(sid, set())
        _require(position not in bucket, f"line {line}: duplicate position {position} in series '{sid}'")
        bucket.add(position)
        series.setdefault(sid, []).append(Observation(sid, position, int(outcome_raw)))

    for obs_list in series.values():
        obs_list.sort(key=lambda o: o.position)
    return series
