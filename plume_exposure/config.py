"""Loading and validation of JSON source/receptor config and met CSV."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass
from pathlib import Path

from .model import DispersionCoefficients


class ConfigError(ValueError):
    """Raised for any invalid user-supplied configuration or met data."""


@dataclass(frozen=True)
class Source:
    id: str
    east_m: float
    north_m: float
    height_m: float
    emission_rate: float  # mass per second


@dataclass(frozen=True)
class Receptor:
    id: str
    east_m: float
    north_m: float
    height_m: float


@dataclass(frozen=True)
class MetPeriod:
    start: str  # ISO 8601
    end: str
    wind_from_deg: float
    wind_speed: float  # m/s, must be > 0
    duration_s: float


@dataclass(frozen=True)
class Scenario:
    decay_rate: float  # 1/s, >= 0 (0 = inert)
    coeffs: DispersionCoefficients
    sources: tuple[Source, ...]
    receptors: tuple[Receptor, ...]


def _finite_number(value: object, what: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{what} must be a number, got {value!r}")
    result = float(value)
    if not math.isfinite(result):
        raise ConfigError(f"{what} must be finite, got {value!r}")
    return result


def _require(condition: bool, message: str) -> None:
    if not condition:
        raise ConfigError(message)


def load_scenario(path: str | Path) -> Scenario:
    data = json.loads(Path(path).read_text(encoding="utf-8-sig"))
    _require(isinstance(data, dict), "scenario root must be a JSON object")

    decay_rate = _finite_number(data.get("decay_rate", 0.0), "decay_rate")
    _require(decay_rate >= 0.0, "decay_rate must be non-negative")

    disp = data.get("dispersion")
    _require(isinstance(disp, dict), "scenario requires a 'dispersion' object")
    coeffs = DispersionCoefficients(
        lateral_coeff=_finite_number(disp.get("lateral_coeff"), "lateral_coeff"),
        lateral_exponent=_finite_number(
            disp.get("lateral_exponent"), "lateral_exponent"
        ),
        vertical_coeff=_finite_number(disp.get("vertical_coeff"), "vertical_coeff"),
        vertical_exponent=_finite_number(
            disp.get("vertical_exponent"), "vertical_exponent"
        ),
    )

    sources = tuple(
        _parse_source(item, index) for index, item in enumerate(_list(data, "sources"))
    )
    receptors = tuple(
        _parse_receptor(item, index)
        for index, item in enumerate(_list(data, "receptors"))
    )
    _require(sources, "scenario must define at least one source")
    _require(receptors, "scenario must define at least one receptor")
    _require(len({s.id for s in sources}) == len(sources), "source ids must be unique")
    _require(
        len({r.id for r in receptors}) == len(receptors),
        "receptor ids must be unique",
    )
    return Scenario(
        decay_rate=decay_rate, coeffs=coeffs, sources=sources, receptors=receptors
    )


def _list(data: dict, key: str) -> list:
    value = data.get(key)
    _require(isinstance(value, list), f"scenario requires a '{key}' array")
    return value


def _parse_source(item: object, index: int) -> Source:
    what = f"sources[{index}]"
    _require(isinstance(item, dict), f"{what} must be an object")
    height = _finite_number(item.get("height_m"), f"{what}.height_m")
    rate = _finite_number(item.get("emission_rate"), f"{what}.emission_rate")
    _require(height >= 0.0, f"{what}.height_m must be non-negative")
    _require(rate >= 0.0, f"{what}.emission_rate must be non-negative")
    return Source(
        id=str(item.get("id", f"source-{index}")),
        east_m=_finite_number(item.get("east_m"), f"{what}.east_m"),
        north_m=_finite_number(item.get("north_m"), f"{what}.north_m"),
        height_m=height,
        emission_rate=rate,
    )


def _parse_receptor(item: object, index: int) -> Receptor:
    what = f"receptors[{index}]"
    _require(isinstance(item, dict), f"{what} must be an object")
    height = _finite_number(item.get("height_m"), f"{what}.height_m")
    _require(height >= 0.0, f"{what}.height_m must be non-negative")
    return Receptor(
        id=str(item.get("id", f"receptor-{index}")),
        east_m=_finite_number(item.get("east_m"), f"{what}.east_m"),
        north_m=_finite_number(item.get("north_m"), f"{what}.north_m"),
        height_m=height,
    )


def load_met_periods(path: str | Path) -> tuple[MetPeriod, ...]:
    """Load met CSV; validate left-closed/right-open non-overlapping periods.

    Required columns: start, end, wind_from_deg, wind_speed.
    Times are ISO 8601 strings. Gaps are allowed; overlaps are rejected.
    """
    from datetime import datetime

    with open(path, newline="", encoding="utf-8-sig") as handle:
        rows = list(csv.DictReader(handle))
    _require(rows, "met CSV contains no data rows")

    parsed: list[tuple[datetime, datetime, MetPeriod]] = []
    for index, row in enumerate(rows):
        what = f"met row {index + 1}"
        try:
            start = datetime.fromisoformat((row.get("start") or "").strip())
            end = datetime.fromisoformat((row.get("end") or "").strip())
        except ValueError as exc:
            raise ConfigError(f"{what}: invalid ISO timestamp: {exc}") from exc
        _require(end > start, f"{what}: end must be after start")
        try:
            wind_from = float((row.get("wind_from_deg") or "").strip())
            wind_speed = float((row.get("wind_speed") or "").strip())
        except ValueError as exc:
            raise ConfigError(f"{what}: wind fields must be numeric") from exc
        _require(math.isfinite(wind_from), f"{what}: wind_from_deg must be finite")
        _require(
            math.isfinite(wind_speed) and wind_speed > 0.0,
            f"{what}: wind_speed must be positive",
        )
        period = MetPeriod(
            start=start.isoformat(),
            end=end.isoformat(),
            wind_from_deg=wind_from,
            wind_speed=wind_speed,
            duration_s=(end - start).total_seconds(),
        )
        parsed.append((start, end, period))

    parsed.sort(key=lambda item: item[0])
    for (prev_start, prev_end, _), (start, end, _) in zip(parsed, parsed[1:]):
        _require(
            start >= prev_end,
            f"met periods overlap: {prev_start.isoformat()}..{prev_end.isoformat()} "
            f"and {start.isoformat()}..{end.isoformat()}",
        )
    return tuple(period for _, _, period in parsed)
