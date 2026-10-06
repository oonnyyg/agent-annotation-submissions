"""Parsing of the JSON job description into model objects."""

from __future__ import annotations

from typing import Any

from .fitting import Candidate, Observation
from .model import (
    Aquifer,
    ModelError,
    ObservationPoint,
    RateStep,
    Well,
    WellFunctionConfig,
)
from .wellfunction import DEFAULT_MAX_TERMS, DEFAULT_TOLERANCE


class InputError(Exception):
    """Raised for malformed job JSON."""


def _require(mapping: dict[str, Any], key: str, context: str) -> Any:
    if key not in mapping:
        raise InputError(f"{context}: missing required key {key!r}")
    return mapping[key]


def _number(value: Any, context: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise InputError(f"{context}: expected a number, got {value!r}")
    return float(value)


def parse_job(data: dict[str, Any]) -> dict[str, Any]:
    if not isinstance(data, dict):
        raise InputError("job file must contain a JSON object")

    aquifer_raw = _require(data, "aquifer", "aquifer")
    try:
        aquifer = Aquifer(
            transmissivity=_number(
                _require(aquifer_raw, "transmissivity", "aquifer"),
                "aquifer.transmissivity",
            ),
            storativity=_number(
                _require(aquifer_raw, "storativity", "aquifer"),
                "aquifer.storativity",
            ),
        )
    except ModelError as exc:
        raise InputError(str(exc)) from exc

    wells: list[Well] = []
    for index, raw in enumerate(data.get("wells", [])):
        context = f"wells[{index}]"
        schedule = tuple(
            RateStep(
                time=_number(_require(step, "time", context), context),
                rate=_number(_require(step, "rate", context), context),
            )
            for step in _require(raw, "schedule", context)
        )
        times = [step.time for step in schedule]
        if times != sorted(times):
            raise InputError(f"{context}: schedule times must be non-decreasing")
        wells.append(
            Well(
                name=str(_require(raw, "name", context)),
                x=_number(_require(raw, "x", context), context),
                y=_number(_require(raw, "y", context), context),
                schedule=schedule,
            )
        )
    if not wells:
        raise InputError("at least one well is required")

    points = [
        ObservationPoint(
            name=str(_require(raw, "name", f"observation_points[{i}]")),
            x=_number(_require(raw, "x", f"observation_points[{i}]"), "x"),
            y=_number(_require(raw, "y", f"observation_points[{i}]"), "y"),
        )
        for i, raw in enumerate(data.get("observation_points", []))
    ]
    if not points:
        raise InputError("at least one observation point is required")

    times = [_number(t, "times") for t in _require(data, "times", "times")]
    if not times:
        raise InputError("times must be a non-empty array")

    wf_raw = data.get("well_function", {})
    config = WellFunctionConfig(
        tolerance=_number(
            wf_raw.get("tolerance", DEFAULT_TOLERANCE), "well_function.tolerance"
        ),
        max_terms=int(wf_raw.get("max_terms", DEFAULT_MAX_TERMS)),
    )

    observations = [
        Observation(
            point=str(_require(raw, "point", f"observations[{i}]")),
            time=_number(_require(raw, "time", f"observations[{i}]"), "time"),
            drawdown=_number(
                _require(raw, "drawdown", f"observations[{i}]"), "drawdown"
            ),
            weight=_number(raw.get("weight", 1.0), "weight"),
        )
        for i, raw in enumerate(data.get("observations", []))
    ]

    candidates = [
        Candidate(
            name=str(_require(raw, "name", f"candidates[{i}]")),
            transmissivity=_number(
                _require(raw, "transmissivity", f"candidates[{i}]"),
                "transmissivity",
            ),
            storativity=_number(
                _require(raw, "storativity", f"candidates[{i}]"), "storativity"
            ),
        )
        for i, raw in enumerate(data.get("candidates", []))
    ]

    return {
        "aquifer": aquifer,
        "wells": wells,
        "points": points,
        "times": times,
        "config": config,
        "observations": observations,
        "candidates": candidates,
    }
