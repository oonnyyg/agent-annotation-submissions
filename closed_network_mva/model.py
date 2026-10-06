"""Input model and JSON loading/validation."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass, field
from pathlib import Path


class ConfigError(ValueError):
    """Raised when the input configuration is invalid."""


@dataclass(frozen=True)
class Station:
    name: str
    kind: str  # "single" or "infinite"
    mean_service_time: float


@dataclass(frozen=True)
class Scenario:
    name: str
    population: int
    service_time_overrides: dict[str, float] = field(default_factory=dict)


@dataclass(frozen=True)
class NetworkConfig:
    stations: list[Station]
    routing: list[list[float]]
    reference_station: str
    scenarios: list[Scenario]

    @property
    def station_index(self) -> dict[str, int]:
        return {s.name: i for i, s in enumerate(self.stations)}


def _require(cond: bool, msg: str) -> None:
    if not cond:
        raise ConfigError(msg)


def _load_stations(raw: object) -> list[Station]:
    _require(isinstance(raw, list) and raw, "'stations' must be a non-empty list")
    stations: list[Station] = []
    seen: set[str] = set()
    for i, item in enumerate(raw):
        _require(isinstance(item, dict), f"stations[{i}] must be an object")
        name = item.get("name")
        _require(isinstance(name, str) and name, f"stations[{i}].name must be a non-empty string")
        _require(name not in seen, f"duplicate station name '{name}'")
        seen.add(name)
        kind = item.get("type")
        _require(kind in ("single", "infinite"),
                 f"station '{name}': type must be 'single' or 'infinite', got {kind!r}")
        st = item.get("mean_service_time")
        _require(isinstance(st, (int, float)) and not isinstance(st, bool) and math.isfinite(st) and st > 0,
                 f"station '{name}': mean_service_time must be a positive finite number, got {st!r}")
        stations.append(Station(name=name, kind=kind, mean_service_time=float(st)))
    return stations


def _load_routing(raw: object, stations: list[Station]) -> list[list[float]]:
    n = len(stations)
    _require(isinstance(raw, list) and len(raw) == n,
             f"'routing' must have exactly {n} rows (one per station)")
    matrix: list[list[float]] = []
    for i, row in enumerate(raw):
        _require(isinstance(row, list) and len(row) == n,
                 f"routing[{i}] ('{stations[i].name}') must have {n} entries")
        out: list[float] = []
        for j, p in enumerate(row):
            _require(isinstance(p, (int, float)) and not isinstance(p, bool) and math.isfinite(p),
                     f"routing[{i}][{j}] must be a finite number, got {p!r}")
            _require(p >= 0.0, f"routing[{i}][{j}] must be non-negative, got {p}")
            out.append(float(p))
        total = sum(out)
        _require(abs(total - 1.0) <= 1e-9,
                 f"routing row {i} ('{stations[i].name}') sums to {total}, expected 1.0")
        matrix.append(out)
    return matrix


def _check_strongly_connected(routing: list[list[float]], stations: list[Station]) -> None:
    n = len(routing)
    adj = [[j for j, p in enumerate(row) if p > 0.0] for row in routing]

    def reachable(start: int, graph: list[list[int]]) -> set[int]:
        seen = {start}
        stack = [start]
        while stack:
            for nxt in graph[stack.pop()]:
                if nxt not in seen:
                    seen.add(nxt)
                    stack.append(nxt)
        return seen

    forward = reachable(0, adj)
    _require(len(forward) == n,
             "routing graph is not strongly connected: unreachable from "
             f"'{stations[0].name}': {[stations[k].name for k in range(n) if k not in forward]}")
    rev = [[i for i in range(n) if routing[i][j] > 0.0] for j in range(n)]
    backward = reachable(0, rev)
    _require(len(backward) == n,
             "routing graph is not strongly connected: cannot reach "
             f"'{stations[0].name}' from {[stations[k].name for k in range(n) if k not in backward]}")


def _load_scenarios(raw: object, known: set[str]) -> list[Scenario]:
    _require(isinstance(raw, list) and raw, "'scenarios' must be a non-empty list")
    scenarios: list[Scenario] = []
    seen: set[str] = set()
    for i, item in enumerate(raw):
        _require(isinstance(item, dict), f"scenarios[{i}] must be an object")
        name = item.get("name")
        _require(isinstance(name, str) and name, f"scenarios[{i}].name must be a non-empty string")
        _require(name not in seen, f"duplicate scenario name '{name}'")
        seen.add(name)
        pop = item.get("population")
        _require(isinstance(pop, int) and not isinstance(pop, bool) and pop >= 1,
                 f"scenario '{name}': population must be a positive integer, got {pop!r}")
        overrides_raw = item.get("service_time_overrides", {})
        _require(isinstance(overrides_raw, dict),
                 f"scenario '{name}': service_time_overrides must be an object")
        overrides: dict[str, float] = {}
        for st_name, value in overrides_raw.items():
            _require(st_name in known,
                     f"scenario '{name}': override targets unknown station '{st_name}'")
            _require(isinstance(value, (int, float)) and not isinstance(value, bool)
                     and math.isfinite(value) and value > 0,
                     f"scenario '{name}': override for '{st_name}' must be a positive finite number")
            overrides[st_name] = float(value)
        scenarios.append(Scenario(name=name, population=pop, service_time_overrides=overrides))
    return scenarios


def load_config(path: str | Path) -> NetworkConfig:
    """Load and validate a network configuration from a JSON file."""
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    _require(isinstance(raw, dict), "top-level JSON value must be an object")
    stations = _load_stations(raw.get("stations"))
    routing = _load_routing(raw.get("routing"), stations)
    _check_strongly_connected(routing, stations)
    ref = raw.get("reference_station")
    names = {s.name for s in stations}
    _require(isinstance(ref, str) and ref in names,
             f"'reference_station' must be one of {sorted(names)}, got {ref!r}")
    scenarios = _load_scenarios(raw.get("scenarios"), names)
    return NetworkConfig(stations=stations, routing=routing,
                         reference_station=ref, scenarios=scenarios)
