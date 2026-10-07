"""Input loading/validation and output writing for the CLI."""

from __future__ import annotations

import csv
import json
import math
from pathlib import Path

from .em import Component
from .vm import TWO_PI, norm_angle


class InputError(ValueError):
    """Raised for any invalid user input; reported as a clean CLI failure."""


def _finite(value: float, what: str) -> float:
    if not math.isfinite(value):
        raise InputError(f"{what} must be finite, got {value!r}")
    return value


def load_observations(path: str | Path) -> tuple[list[float], list[float]]:
    """Load a CSV with columns `angle` and `weight` (header required).

    Weights must be positive and finite; angles must be finite. Angles are
    returned raw; unit conversion happens after the config is known.
    """
    angles: list[float] = []
    weights: list[float] = []
    try:
        with open(path, newline="", encoding="utf-8") as fh:
            reader = csv.DictReader(fh)
            if reader.fieldnames is None or not {
                "angle",
                "weight",
            } <= set(reader.fieldnames):
                raise InputError(
                    "CSV must have header columns 'angle' and 'weight'"
                )
            for lineno, row in enumerate(reader, start=2):
                try:
                    angle = float(row["angle"])
                    weight = float(row["weight"])
                except (TypeError, ValueError) as exc:
                    raise InputError(
                        f"line {lineno}: angle/weight must be numbers"
                    ) from exc
                _finite(angle, f"line {lineno} angle")
                _finite(weight, f"line {lineno} weight")
                if weight <= 0.0:
                    raise InputError(
                        f"line {lineno}: weight must be positive, got {weight}"
                    )
                angles.append(angle)
                weights.append(weight)
    except OSError as exc:
        raise InputError(f"cannot read observations CSV: {exc}") from exc
    if not angles:
        raise InputError("observations CSV is empty")
    return angles, weights


def load_config(path: str | Path) -> dict:
    """Load and validate the JSON config. Returns a normalized dict with
    keys: unit, tol, max_iter, components (list of Component)."""
    try:
        with open(path, encoding="utf-8") as fh:
            cfg = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        raise InputError(f"cannot read config JSON: {exc}") from exc
    if not isinstance(cfg, dict):
        raise InputError("config must be a JSON object")

    unit = cfg.get("unit", "degrees")
    if unit not in ("degrees", "radians"):
        raise InputError("config 'unit' must be 'degrees' or 'radians'")

    tol = float(cfg.get("tolerance", 1e-8))
    _finite(tol, "tolerance")
    if tol <= 0.0:
        raise InputError("tolerance must be positive")
    max_iter = int(cfg.get("max_iterations", 500))
    if max_iter < 1:
        raise InputError("max_iterations must be >= 1")

    raw = cfg.get("components")
    if not isinstance(raw, list) or not 1 <= len(raw) <= 3:
        raise InputError("config 'components' must list 1 to 3 components")

    scale = math.pi / 180.0 if unit == "degrees" else 1.0
    components: list[Component] = []
    for i, comp in enumerate(raw):
        if not isinstance(comp, dict):
            raise InputError(f"component {i} must be an object")
        try:
            weight = float(comp["weight"])
            mu = float(comp["mu"])
            kappa = float(comp["kappa"])
        except (KeyError, TypeError, ValueError) as exc:
            raise InputError(
                f"component {i} needs numeric 'weight', 'mu', 'kappa'"
            ) from exc
        _finite(weight, f"component {i} weight")
        _finite(mu, f"component {i} mu")
        _finite(kappa, f"component {i} kappa")
        if weight < 0.0:
            raise InputError(f"component {i} weight must be non-negative")
        if kappa < 0.0:
            raise InputError(f"component {i} kappa must be non-negative")
        components.append(Component(weight, norm_angle(mu * scale), kappa))

    wsum = sum(c.weight for c in components)
    if wsum <= 0.0:
        raise InputError("component weights must not all be zero")
    for c in components:
        c.weight /= wsum

    return {
        "unit": unit,
        "tolerance": tol,
        "max_iterations": max_iter,
        "components": components,
    }


def to_radians(angles: list[float], unit: str) -> list[float]:
    """Convert declared-unit angles to radians normalized to [0, 2*pi)."""
    scale = math.pi / 180.0 if unit == "degrees" else 1.0
    return [norm_angle(a * scale) for a in angles]
