"""Input parsing and validation for the transport problem."""

from __future__ import annotations

import csv
import json
from dataclasses import dataclass, field
from pathlib import Path

MAX_LOCATIONS = 32
METRICS = ("manhattan", "sqeuclidean")


class InputError(ValueError):
    """Raised when user-supplied input data is invalid."""


@dataclass(frozen=True)
class Location:
    ident: str
    x: int
    y: int
    mass: int


@dataclass(frozen=True)
class Distribution:
    locations: tuple[Location, ...]

    @property
    def total_mass(self) -> int:
        return sum(loc.mass for loc in self.locations)


@dataclass(frozen=True)
class Problem:
    sources: Distribution
    targets: Distribution
    metric: str
    disabled: frozenset[tuple[str, str]] = field(default_factory=frozenset)


def _parse_int(text: str, what: str) -> int:
    try:
        return int(text)
    except (TypeError, ValueError):
        raise InputError(f"{what} must be an integer, got {text!r}") from None


def load_distribution(path: str | Path, role: str) -> Distribution:
    """Load a CSV with columns id,x,y,mass (header required)."""
    locations: list[Location] = []
    seen: set[str] = set()
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.DictReader(fh)
        required = {"id", "x", "y", "mass"}
        if reader.fieldnames is None or not required.issubset(
            {name.strip() for name in reader.fieldnames}
        ):
            raise InputError(
                f"{role} CSV must have header columns id,x,y,mass"
            )
        for lineno, row in enumerate(reader, start=2):
            ident = (row.get("id") or "").strip()
            if not ident:
                raise InputError(f"{role} CSV line {lineno}: empty id")
            if ident in seen:
                raise InputError(
                    f"{role} CSV line {lineno}: duplicate id {ident!r}"
                )
            seen.add(ident)
            x = _parse_int((row.get("x") or "").strip(), f"{role} line {lineno} x")
            y = _parse_int((row.get("y") or "").strip(), f"{role} line {lineno} y")
            mass = _parse_int(
                (row.get("mass") or "").strip(), f"{role} line {lineno} mass"
            )
            if mass < 0:
                raise InputError(
                    f"{role} CSV line {lineno}: negative mass {mass}"
                )
            locations.append(Location(ident, x, y, mass))
    if not locations:
        raise InputError(f"{role} CSV contains no locations")
    if len(locations) > MAX_LOCATIONS:
        raise InputError(
            f"{role} has {len(locations)} locations; maximum is {MAX_LOCATIONS}"
        )
    return Distribution(tuple(locations))


def load_config(path: str | Path) -> tuple[str, frozenset[tuple[str, str]]]:
    """Load JSON config: {"metric": ..., "disabled": [[src, dst], ...]}."""
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except json.JSONDecodeError as exc:
        raise InputError(f"config is not valid JSON: {exc}") from None
    if not isinstance(data, dict):
        raise InputError("config JSON must be an object")
    metric = data.get("metric", "manhattan")
    if metric not in METRICS:
        raise InputError(
            f"unknown metric {metric!r}; choose from {', '.join(METRICS)}"
        )
    disabled_raw = data.get("disabled", [])
    if not isinstance(disabled_raw, list):
        raise InputError("config 'disabled' must be a list of [source, target] pairs")
    disabled: set[tuple[str, str]] = set()
    for entry in disabled_raw:
        if (
            not isinstance(entry, list)
            or len(entry) != 2
            or not all(isinstance(v, str) for v in entry)
        ):
            raise InputError(
                f"disabled entry must be [source_id, target_id], got {entry!r}"
            )
        disabled.add((entry[0], entry[1]))
    return metric, frozenset(disabled)


def load_problem(
    source_csv: str | Path, target_csv: str | Path, config_json: str | Path
) -> Problem:
    sources = load_distribution(source_csv, "source")
    targets = load_distribution(target_csv, "target")
    metric, disabled = load_config(config_json)
    source_ids = {loc.ident for loc in sources.locations}
    target_ids = {loc.ident for loc in targets.locations}
    for src, dst in sorted(disabled):
        if src not in source_ids:
            raise InputError(f"disabled pair references unknown source {src!r}")
        if dst not in target_ids:
            raise InputError(f"disabled pair references unknown target {dst!r}")
    if sources.total_mass != targets.total_mass:
        raise InputError(
            f"total mass mismatch: sources {sources.total_mass} "
            f"vs targets {targets.total_mass}"
        )
    if sources.total_mass <= 0:
        raise InputError("total mass must be positive")
    return Problem(sources, targets, metric, disabled)
