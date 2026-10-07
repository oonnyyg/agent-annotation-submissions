"""CSV/JSON input parsing and validation."""

from __future__ import annotations

import csv
import json
import math
from pathlib import Path

from .em import Component
from .errors import InputError
from .vmm import KAPPA_MAX, normalize_angle


def load_observations(path: str | Path, unit: str) -> tuple[list[float], list[float]]:
    """Read a CSV with columns angle,weight. Angles are normalized to [0, 2pi)."""
    if unit not in ("deg", "rad"):
        raise InputError(f"unit must be 'deg' or 'rad', got {unit!r}")
    scale = math.pi / 180.0 if unit == "deg" else 1.0
    angles: list[float] = []
    weights: list[float] = []
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.DictReader(fh)
        if reader.fieldnames is None or not {"angle", "weight"} <= set(reader.fieldnames):
            raise InputError("CSV must have header columns 'angle' and 'weight'")
        for lineno, row in enumerate(reader, start=2):
            try:
                angle = float(row["angle"])
                weight = float(row["weight"])
            except (TypeError, ValueError):
                raise InputError(f"line {lineno}: angle/weight must be numbers") from None
            if not math.isfinite(angle):
                raise InputError(f"line {lineno}: angle is not finite")
            if not math.isfinite(weight) or weight <= 0.0:
                raise InputError(f"line {lineno}: weight must be positive and finite")
            angles.append(normalize_angle(angle * scale))
            weights.append(weight)
    if not angles:
        raise InputError("CSV contains no observations")
    return angles, weights


def load_config(path: str | Path) -> dict:
    """Read and validate the JSON configuration."""
    try:
        with open(path, encoding="utf-8") as fh:
            cfg = json.load(fh)
    except json.JSONDecodeError as exc:
        raise InputError(f"invalid JSON config: {exc}") from None
    if not isinstance(cfg, dict):
        raise InputError("config must be a JSON object")

    unit = cfg.get("unit", "deg")
    if unit not in ("deg", "rad"):
        raise InputError("config 'unit' must be 'deg' or 'rad'")
    scale = math.pi / 180.0 if unit == "deg" else 1.0

    raw_components = cfg.get("components")
    if not isinstance(raw_components, list) or not (1 <= len(raw_components) <= 3):
        raise InputError("config 'components' must be a list of 1 to 3 entries")
    components: list[Component] = []
    for idx, raw in enumerate(raw_components):
        try:
            weight = float(raw["weight"])
            mu = float(raw["mu"])
            kappa = float(raw["kappa"])
        except (KeyError, TypeError, ValueError):
            raise InputError(
                f"component {idx}: requires numeric 'weight', 'mu', 'kappa'"
            ) from None
        if not math.isfinite(weight) or weight < 0.0:
            raise InputError(f"component {idx}: weight must be non-negative and finite")
        if not math.isfinite(mu):
            raise InputError(f"component {idx}: mu must be finite")
        if not math.isfinite(kappa) or not (0.0 <= kappa <= KAPPA_MAX):
            raise InputError(f"component {idx}: kappa must be in [0, {KAPPA_MAX}]")
        components.append(
            Component(weight=weight, mu=normalize_angle(mu * scale), kappa=kappa)
        )
    w_sum = sum(c.weight for c in components)
    if w_sum <= 0.0:
        raise InputError("component weights must not all be zero")
    for c in components:
        c.weight /= w_sum

    tol = float(cfg.get("tolerance", 1e-8))
    max_iter = int(cfg.get("max_iterations", 500))
    if not math.isfinite(tol) or tol <= 0.0:
        raise InputError("config 'tolerance' must be positive")
    if max_iter < 1:
        raise InputError("config 'max_iterations' must be >= 1")

    return {
        "unit": unit,
        "components": components,
        "tolerance": tol,
        "max_iterations": max_iter,
    }
