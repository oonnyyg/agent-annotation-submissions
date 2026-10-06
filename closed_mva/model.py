"""Data model and JSON input validation for the closed-network MVA tool."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass, field
from pathlib import Path


class InputError(ValueError):
    """Raised when the JSON input is invalid; message pinpoints the cause."""


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
class Network:
    stations: list[Station]
    routing: dict[str, dict[str, float]]
    reference_station: str
    scenarios: list[Scenario]

    def station_names(self) -> list[str]:
        return [s.name for s in self.stations]


def _fail(where: str, why: str) -> None:
    raise InputError(f"{where}: {why}")


def _load_stations(raw: object) -> list[Station]:
    if not isinstance(raw, list) or not raw:
        _fail("stations", "must be a non-empty list")
    stations: list[Station] = []
    seen: set[str] = set()
    for i, item in enumerate(raw):
        where = f"stations[{i}]"
        if not isinstance(item, dict):
            _fail(where, "must be an object")
        name = item.get("name")
        if not isinstance(name, str) or not name:
            _fail(where, "'name' must be a non-empty string")
        if name in seen:
            _fail(where, f"duplicate station name {name!r}")
        seen.add(name)
        kind = item.get("type")
        if kind not in ("single", "infinite"):
            _fail(where, f"'type' must be 'single' or 'infinite', got {kind!r}")
        s = item.get("mean_service_time")
        if not isinstance(s, (int, float)) or isinstance(s, bool) or not math.isfinite(s) or s <= 0:
            _fail(where, f"'mean_service_time' must be a positive finite number, got {s!r}")
        stations.append(Station(name=name, kind=kind, mean_service_time=float(s)))
    return stations


def _load_routing(raw: object, names: list[str]) -> dict[str, dict[str, float]]:
    if not isinstance(raw, dict):
        _fail("routing", "must be an object mapping station -> {station: probability}")
    known = set(names)
    routing: dict[str, dict[str, float]] = {}
    for src in names:
        row = raw.get(src)
        where = f"routing[{src!r}]"
        if not isinstance(row, dict):
            _fail(where, "missing or not an object; every station needs a routing row")
        total = 0.0
        out: dict[str, float] = {}
        for dst, p in row.items():
            if dst not in known:
                _fail(where, f"unknown destination station {dst!r}")
            if not isinstance(p, (int, float)) or isinstance(p, bool) or not math.isfinite(p) or p < 0:
                _fail(where, f"probability to {dst!r} must be a non-negative finite number, got {p!r}")
            if p > 0:
                out[dst] = float(p)
            total += p
        if abs(total - 1.0) > 1e-9:
            _fail(where, f"probabilities sum to {total}, expected 1.0")
        routing[src] = out
    extra = set(raw) - known
    if extra:
        _fail("routing", f"rows for unknown stations: {sorted(extra)}")
    return routing


def _check_strongly_connected(names: list[str], routing: dict[str, dict[str, float]]) -> None:
    def reach(start: str, graph: dict[str, dict[str, float]]) -> set[str]:
        seen = {start}
        stack = [start]
        while stack:
            for nxt in graph[stack.pop()]:
                if nxt not in seen:
                    seen.add(nxt)
                    stack.append(nxt)
        return seen

    first = names[0]
    forward = reach(first, routing)
    if len(forward) != len(names):
        _fail("routing", f"not strongly connected; unreachable from {first!r}: {sorted(set(names) - forward)}")
    reverse = {n: {} for n in names}
    for src, row in routing.items():
        for dst, p in row.items():
            reverse[dst][src] = p
    backward = reach(first, reverse)
    if len(backward) != len(names):
        _fail("routing", f"not strongly connected; cannot reach {first!r} from: {sorted(set(names) - backward)}")


def _load_scenarios(raw: object, names: list[str]) -> list[Scenario]:
    if not isinstance(raw, list) or not raw:
        _fail("scenarios", "must be a non-empty list")
    known = set(names)
    scenarios: list[Scenario] = []
    seen: set[str] = set()
    for i, item in enumerate(raw):
        where = f"scenarios[{i}]"
        if not isinstance(item, dict):
            _fail(where, "must be an object")
        name = item.get("name", f"scenario-{i}")
        if not isinstance(name, str) or not name:
            _fail(where, "'name' must be a non-empty string")
        if name in seen:
            _fail(where, f"duplicate scenario name {name!r}")
        seen.add(name)
        pop = item.get("population")
        if not isinstance(pop, int) or isinstance(pop, bool) or pop < 0:
            _fail(where, f"'population' must be a non-negative integer, got {pop!r}")
        overrides_raw = item.get("service_time_overrides", {})
        if not isinstance(overrides_raw, dict):
            _fail(where, "'service_time_overrides' must be an object")
        overrides: dict[str, float] = {}
        for st, val in overrides_raw.items():
            if st not in known:
                _fail(where, f"override targets unknown station {st!r}")
            if not isinstance(val, (int, float)) or isinstance(val, bool) or not math.isfinite(val) or val <= 0:
                _fail(where, f"override for {st!r} must be a positive finite number, got {val!r}")
            overrides[st] = float(val)
        scenarios.append(Scenario(name=name, population=pop, service_time_overrides=overrides))
    return scenarios


def load_network(path: str | Path) -> Network:
    """Load and validate a network definition from a JSON file."""
    try:
        raw = json.loads(Path(path).read_text(encoding="utf-8"))
    except json.JSONDecodeError as exc:
        raise InputError(f"{path}: invalid JSON at line {exc.lineno} column {exc.colno}: {exc.msg}") from exc
    if not isinstance(raw, dict):
        _fail("root", "must be a JSON object")
    stations = _load_stations(raw.get("stations"))
    names = [s.name for s in stations]
    routing = _load_routing(raw.get("routing"), names)
    _check_strongly_connected(names, routing)
    ref = raw.get("reference_station")
    if not isinstance(ref, str) or ref not in set(names):
        _fail("reference_station", f"must be one of {names}, got {ref!r}")
    scenarios = _load_scenarios(raw.get("scenarios"), names)
    return Network(stations=stations, routing=routing, reference_station=ref, scenarios=scenarios)
