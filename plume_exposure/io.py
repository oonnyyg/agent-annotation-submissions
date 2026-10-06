"""Loading and validation of JSON configuration and meteorology CSV."""

from __future__ import annotations

import csv
import json
from dataclasses import dataclass
from datetime import datetime
from pathlib import Path


class ConfigError(ValueError):
    """Raised when configuration or meteorology input is invalid."""


@dataclass(frozen=True)
class Source:
    id: str
    easting: float
    northing: float
    height: float
    emission_rate: float  # mass per second


@dataclass(frozen=True)
class Receptor:
    id: str
    easting: float
    northing: float
    height: float


@dataclass(frozen=True)
class Dispersion:
    sigma_y_coeff: float
    sigma_y_exp: float
    sigma_z_coeff: float
    sigma_z_exp: float


@dataclass(frozen=True)
class Config:
    pollutant: str
    decay_rate: float  # 1/s
    sources: list[Source]
    receptors: list[Receptor]
    dispersion: Dispersion


@dataclass(frozen=True)
class MetPeriod:
    start: datetime
    end: datetime
    wind_speed: float  # m/s, must be > 0
    wind_dir: float  # degrees, direction wind comes FROM, clockwise from north

    @property
    def duration(self) -> float:
        return (self.end - self.start).total_seconds()


def _require_number(value: object, name: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ConfigError(f"{name} must be a number, got {value!r}")
    return float(value)


def load_config(path: str | Path) -> Config:
    try:
        data = json.loads(Path(path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise ConfigError(f"cannot read config {path}: {exc}") from exc

    pollutant = data.get("pollutant")
    if isinstance(pollutant, dict):
        name = pollutant.get("name")
        decay = _require_number(pollutant.get("decay_rate", 0.0), "pollutant.decay_rate")
    else:
        name = pollutant
        decay = _require_number(data.get("decay_rate", 0.0), "decay_rate")
    if not isinstance(name, str) or not name:
        raise ConfigError("pollutant name must be a non-empty string")
    if decay < 0.0:
        raise ConfigError("decay_rate must be non-negative")

    disp_raw = data.get("dispersion")
    if not isinstance(disp_raw, dict):
        raise ConfigError("config requires a 'dispersion' object")
    disp = Dispersion(
        sigma_y_coeff=_require_number(disp_raw.get("sigma_y_coeff"), "sigma_y_coeff"),
        sigma_y_exp=_require_number(disp_raw.get("sigma_y_exp"), "sigma_y_exp"),
        sigma_z_coeff=_require_number(disp_raw.get("sigma_z_coeff"), "sigma_z_coeff"),
        sigma_z_exp=_require_number(disp_raw.get("sigma_z_exp"), "sigma_z_exp"),
    )
    for field_name, value in (
        ("sigma_y_coeff", disp.sigma_y_coeff),
        ("sigma_y_exp", disp.sigma_y_exp),
        ("sigma_z_coeff", disp.sigma_z_coeff),
        ("sigma_z_exp", disp.sigma_z_exp),
    ):
        if value <= 0.0:
            raise ConfigError(f"{field_name} must be positive")

    sources = _load_points(data, "sources", Source, ("emission_rate",))
    for src in sources:
        if src.emission_rate < 0.0:
            raise ConfigError(f"source {src.id!r} emission_rate must be non-negative")
    receptors = _load_points(data, "receptors", Receptor, ())
    return Config(name, decay, sources, receptors, disp)


def _load_points(data, kind, cls, extra):
    items = data.get(kind)
    if not isinstance(items, list) or not items:
        raise ConfigError(f"config requires a non-empty '{kind}' list")
    out = []
    seen = set()
    for i, raw in enumerate(items):
        if not isinstance(raw, dict):
            raise ConfigError(f"{kind}[{i}] must be an object")
        pid = raw.get("id")
        if not isinstance(pid, str) or not pid:
            raise ConfigError(f"{kind}[{i}].id must be a non-empty string")
        if pid in seen:
            raise ConfigError(f"duplicate {kind} id {pid!r}")
        seen.add(pid)
        easting = _require_number(raw.get("easting"), f"{kind}[{i}].easting")
        northing = _require_number(raw.get("northing"), f"{kind}[{i}].northing")
        height = _require_number(raw.get("height"), f"{kind}[{i}].height")
        if height < 0.0:
            raise ConfigError(f"{kind}[{i}].height must be non-negative")
        values = {"id": pid, "easting": easting, "northing": northing, "height": height}
        for field_name in extra:
            values[field_name] = _require_number(
                raw.get(field_name), f"{kind}[{i}].{field_name}"
            )
        out.append(cls(**values))
    return out


def _parse_time(text: str, context: str) -> datetime:
    try:
        return datetime.fromisoformat(text.strip())
    except ValueError as exc:
        raise ConfigError(f"{context}: invalid ISO timestamp {text!r}") from exc


def load_met_periods(path: str | Path) -> list[MetPeriod]:
    """Load meteorology periods; reject the whole file on any invalid row.

    Periods are half-open [start, end). Gaps are allowed; overlaps are not.
    """
    try:
        with open(path, newline="", encoding="utf-8") as fh:
            rows = list(csv.DictReader(fh))
    except OSError as exc:
        raise ConfigError(f"cannot read met file {path}: {exc}") from exc
    if not rows:
        raise ConfigError("met file has no data rows")

    periods: list[MetPeriod] = []
    for i, row in enumerate(rows):
        ctx = f"met row {i + 1}"
        try:
            start = _parse_time(row["start"], ctx)
            end = _parse_time(row["end"], ctx)
            wind_speed = float(row["wind_speed"])
            wind_dir = float(row["wind_dir"])
        except KeyError as exc:
            raise ConfigError(f"{ctx}: missing column {exc}") from exc
        except ValueError as exc:
            raise ConfigError(f"{ctx}: {exc}") from exc
        if end <= start:
            raise ConfigError(f"{ctx}: end must be after start")
        if wind_speed <= 0.0:
            raise ConfigError(f"{ctx}: wind_speed must be positive")
        if not 0.0 <= wind_dir < 360.0:
            raise ConfigError(f"{ctx}: wind_dir must be in [0, 360)")
        periods.append(MetPeriod(start, end, wind_speed, wind_dir))

    periods.sort(key=lambda p: p.start)
    for prev, cur in zip(periods, periods[1:]):
        if cur.start < prev.end:
            raise ConfigError(
                f"overlapping met periods: {prev.start}..{prev.end} and "
                f"{cur.start}..{cur.end}"
            )
    return periods
