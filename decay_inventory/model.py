"""Network model loading and validation."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

MAX_NUCLIDES = 12
LN2 = math.log(2.0)


class ModelError(ValueError):
    """Raised when the decay network or schedule is invalid."""


@dataclass
class Nuclide:
    name: str
    half_life: float | None  # None means stable
    initial: float

    @property
    def stable(self) -> bool:
        return self.half_life is None

    @property
    def decay_constant(self) -> float:
        return 0.0 if self.stable else LN2 / self.half_life


@dataclass
class Feed:
    target: int  # nuclide index
    rate: float  # atoms per unit time, >= 0
    start: float
    end: float  # half-open interval [start, end)


@dataclass
class Network:
    nuclides: list[Nuclide]
    branches: list[list[tuple[int, float]]]  # branches[i] = [(child, prob)]
    feeds: list[Feed] = field(default_factory=list)

    @property
    def size(self) -> int:
        return len(self.nuclides)


def _require(cond: bool, message: str) -> None:
    if not cond:
        raise ModelError(message)


def load_network(data: dict) -> Network:
    _require(isinstance(data, dict), "network document must be a JSON object")
    raw_nuclides = data.get("nuclides")
    _require(isinstance(raw_nuclides, list) and raw_nuclides,
             "'nuclides' must be a non-empty list")
    _require(len(raw_nuclides) <= MAX_NUCLIDES,
             f"at most {MAX_NUCLIDES} nuclides are supported")

    nuclides: list[Nuclide] = []
    names: set[str] = set()
    for entry in raw_nuclides:
        _require(isinstance(entry, dict), "each nuclide must be an object")
        name = entry.get("name")
        _require(isinstance(name, str) and name, "nuclide requires a name")
        _require(name not in names, f"duplicate nuclide name {name!r}")
        names.add(name)
        half_life = entry.get("half_life")
        if half_life is not None:
            _require(isinstance(half_life, (int, float))
                     and math.isfinite(half_life) and half_life > 0.0,
                     f"nuclide {name!r}: half_life must be a positive number")
            half_life = float(half_life)
        initial = entry.get("initial", 0.0)
        _require(isinstance(initial, (int, float)) and math.isfinite(initial)
                 and initial >= 0.0,
                 f"nuclide {name!r}: initial quantity must be a non-negative number")
        nuclides.append(Nuclide(name, half_life, float(initial)))

    index = {n.name: i for i, n in enumerate(nuclides)}
    branches: list[list[tuple[int, float]]] = [[] for _ in nuclides]
    for edge in data.get("edges", []):
        _require(isinstance(edge, dict), "each edge must be an object")
        parent = edge.get("parent")
        child = edge.get("child")
        _require(parent in index, f"edge parent {parent!r} is not a nuclide")
        _require(child in index, f"edge child {child!r} is not a nuclide")
        prob = edge.get("probability")
        _require(isinstance(prob, (int, float)) and math.isfinite(prob)
                 and prob > 0.0,
                 f"edge {parent!r}->{child!r}: probability must be positive")
        _require(not nuclides[index[parent]].stable,
                 f"stable nuclide {parent!r} must not have outgoing edges")
        _require(parent != child, f"self-decay edge on {parent!r} is not allowed")
        branches[index[parent]].append((index[child], float(prob)))

    for i, nuclide in enumerate(nuclides):
        if nuclide.stable:
            _require(not branches[i],
                     f"stable nuclide {nuclide.name!r} must not have outgoing edges")
        else:
            _require(branches[i],
                     f"unstable nuclide {nuclide.name!r} needs at least one branch")
            total = sum(p for _, p in branches[i])
            _require(abs(total - 1.0) <= 1e-9,
                     f"branches of {nuclide.name!r} sum to {total!r}, expected 1.0")

    _check_acyclic(branches, nuclides)

    feeds = _load_feeds(data.get("feeds", []), index)
    return Network(nuclides, branches, feeds)


def _check_acyclic(branches: list[list[tuple[int, float]]],
                   nuclides: list[Nuclide]) -> None:
    color = [0] * len(nuclides)  # 0=white, 1=gray, 2=black

    def visit(node: int) -> None:
        _require(color[node] != 1,
                 f"decay network must be acyclic (cycle at {nuclides[node].name!r})")
        if color[node] == 2:
            return
        color[node] = 1
        for child, _ in branches[node]:
            visit(child)
        color[node] = 2

    for node in range(len(nuclides)):
        if color[node] == 0:
            visit(node)


def _load_feeds(raw_feeds: object, index: dict[str, int]) -> list[Feed]:
    _require(isinstance(raw_feeds, list), "'feeds' must be a list")
    feeds: list[Feed] = []
    for entry in raw_feeds:
        _require(isinstance(entry, dict), "each feed must be an object")
        name = entry.get("nuclide")
        _require(name in index, f"feed nuclide {name!r} is not defined")
        rate = entry.get("rate")
        _require(isinstance(rate, (int, float)) and math.isfinite(rate)
                 and rate >= 0.0,
                 f"feed for {name!r}: rate must be a non-negative number")
        start = entry.get("start")
        end = entry.get("end")
        for label, value in (("start", start), ("end", end)):
            _require(isinstance(value, (int, float)) and math.isfinite(value),
                     f"feed for {name!r}: {label} must be a finite number")
        _require(end > start,
                 f"feed for {name!r}: end must be greater than start")
        feeds.append(Feed(index[name], float(rate), float(start), float(end)))
    # Half-open intervals must not overlap per nuclide.
    per_nuclide: dict[int, list[Feed]] = {}
    for feed in feeds:
        per_nuclide.setdefault(feed.target, []).append(feed)
    for target, group in per_nuclide.items():
        group.sort(key=lambda f: f.start)
        for earlier, later in zip(group, group[1:]):
            _require(later.start >= earlier.end,
                     "feed intervals for one nuclide must not overlap")
    return feeds
