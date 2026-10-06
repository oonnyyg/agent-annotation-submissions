
"""JSON input parsing and validation."""

from __future__ import annotations

import json
from dataclasses import dataclass, field

from .model import AquiferModel, ModelError, ObservationPoint, Well


class ConfigError(Exception):
    """Raised for malformed or inconsistent JSON input."""


@dataclass
class ObservedSeries:
    point: str
    times: list[float]
    drawdowns: list[float]
    weights: list[float]


@dataclass
class Candidate:
    name: str
    transmissivity: float
    storativity: float


@dataclass
class Config:
    model: AquiferModel
    points: list[ObservationPoint]
    times: list[float]
    observed: list[ObservedSeries] = field(default_factory=list)
    candidates: list[Candidate] = field(default_factory=list)


def _num(value, what: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{what} must be a number, got {value!r}")
    return float(value)


def load_config(path: str) -> Config:
    with open(path, "r", encoding="utf-8") as fh:
        try:
            data = json.load(fh)
        except json.JSONDecodeError as exc:
            raise ConfigError(f"invalid JSON in {path}: {exc}") from exc
    if not isinstance(data, dict):
        raise ConfigError("top-level JSON value must be an object")

    try:
        aquifer = data["aquifer"]
        transmissivity = _num(aquifer["transmissivity"], "transmissivity")
        storativity = _num(aquifer["storativity"], "storativity")
    except KeyError as exc:
        raise ConfigError(f"missing required key: {exc}") from exc

    wf = data.get("well_function", {})
    tol = _num(wf.get("tolerance", 1e-12), "well_function.tolerance")
    max_terms = int(wf.get("max_terms", 10000))

    wells: list[Well] = []
    for i, w in enumerate(data.get("wells", [])):
        name = str(w.get("name", f"well_{i}"))
        schedule = tuple(
            (_num(entry[0], "schedule time"), _num(entry[1], "schedule rate"))
            for entry in w.get("schedule", [])
        )
        wells.append(
            Well(
                name=name,
                x=_num(w.get("x"), f"well {name} x"),
                y=_num(w.get("y"), f"well {name} y"),
                schedule=schedule,
            )
        )

    points = [
        ObservationPoint(
            name=str(p.get("name", f"obs_{i}")),
            x=_num(p.get("x"), "observation x"),
            y=_num(p.get("y"), "observation y"),
        )
        for i, p in enumerate(data.get("observation_points", []))
    ]
    if not points:
        raise ConfigError("at least one observation point is required")

    times = [_num(t, "output time") for t in data.get("times", [])]
    if not times:
        raise ConfigError("at least one output time is required")
    if any(t2 <= t1 for t1, t2 in zip(times, times[1:])):
        raise ConfigError("output times must be strictly increasing")

    observed = []
    for o in data.get("observed", []):
        weights = [_num(w, "weight") for w in o.get("weights", [])]
        if any(w <= 0.0 for w in weights):
            raise ConfigError("observation weights must be positive")
        n = len(o.get("times", []))
        if len(o.get("drawdowns", [])) != n or len(weights) != n:
            raise ConfigError(
                "observed times, drawdowns and weights must have equal length"
            )
        observed.append(
            ObservedSeries(
                point=str(o["point"]),
                times=[_num(t, "observed time") for t in o["times"]],
                drawdowns=[_num(d, "observed drawdown") for d in o["drawdowns"]],
                weights=weights,
            )
        )

    candidates = [
        Candidate(
            name=str(c.get("name", f"candidate_{i}")),
            transmissivity=_num(c["transmissivity"], "candidate transmissivity"),
            storativity=_num(c["storativity"], "candidate storativity"),
        )
        for i, c in enumerate(data.get("candidates", []))
    ]

    try:
        model = AquiferModel(
            transmissivity=transmissivity,
            storativity=storativity,
            wells=wells,
            tol=tol,
            max_terms=max_terms,
        )
        model.validate_points(points)
    except ModelError as exc:
        raise ConfigError(str(exc)) from exc

    return Config(
        model=model, points=points, times=times,
        observed=observed, candidates=candidates,
    )
