"""Network and scenario data model with validation."""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from pathlib import Path


class ModelError(ValueError):
    """Raised when the network or scenario definition is invalid."""


@dataclass(frozen=True)
class Pipe:
    """A directed pipe: positive flow goes from `start` to `end`.

    Head drop along the positive direction: h_start - h_end = r * q * |q|.
    """

    name: str
    start: str
    end: str
    resistance: float  # r > 0, units: head / (flow^2)


@dataclass(frozen=True)
class Network:
    fixed_heads: dict[str, float]  # node -> fixed head (boundary)
    pipes: tuple[Pipe, ...]

    @property
    def nodes(self) -> set[str]:
        nodes = set(self.fixed_heads)
        for pipe in self.pipes:
            nodes.add(pipe.start)
            nodes.add(pipe.end)
        return nodes

    @property
    def demand_nodes(self) -> list[str]:
        return sorted(self.nodes - set(self.fixed_heads))


@dataclass(frozen=True)
class Scenario:
    """One independent loading case.

    demands: node -> net withdrawal (positive = outflow, negative = injection).
    Nodes absent from the mapping have zero demand.
    """

    name: str
    demands: dict[str, float] = field(default_factory=dict)


def _require_number(value: object, what: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ModelError(f"{what} must be a number, got {value!r}")
    return float(value)


def load_network(path: str | Path) -> Network:
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(data, dict):
        raise ModelError("network file must contain a JSON object")

    raw_heads = data.get("fixed_heads")
    if not isinstance(raw_heads, dict) or not raw_heads:
        raise ModelError("network requires a non-empty 'fixed_heads' object")
    fixed_heads = {
        str(node): _require_number(head, f"fixed head of node {node!r}")
        for node, head in raw_heads.items()
    }

    raw_pipes = data.get("pipes")
    if not isinstance(raw_pipes, list) or not raw_pipes:
        raise ModelError("network requires a non-empty 'pipes' array")

    pipes: list[Pipe] = []
    seen_names: set[str] = set()
    for index, item in enumerate(raw_pipes):
        if not isinstance(item, dict):
            raise ModelError(f"pipe #{index} must be an object")
        try:
            name = str(item["name"])
            start = str(item["from"])
            end = str(item["to"])
        except KeyError as exc:
            raise ModelError(f"pipe #{index} missing key {exc}") from None
        resistance = _require_number(item.get("resistance"), f"resistance of pipe {name!r}")
        if resistance <= 0.0:
            raise ModelError(f"pipe {name!r}: resistance must be positive")
        if start == end:
            raise ModelError(f"pipe {name!r}: self-loops are not allowed")
        if name in seen_names:
            raise ModelError(f"duplicate pipe name {name!r}")
        seen_names.add(name)
        pipes.append(Pipe(name=name, start=start, end=end, resistance=resistance))

    network = Network(fixed_heads=fixed_heads, pipes=tuple(pipes))
    _check_fixed_head_reachability(network)
    return network


def _check_fixed_head_reachability(network: Network) -> None:
    """Every connected component must contain at least one fixed-head node."""
    parent = {node: node for node in network.nodes}

    def find(node: str) -> str:
        while parent[node] != node:
            parent[node] = parent[parent[node]]
            node = parent[node]
        return node

    for pipe in network.pipes:
        ra, rb = find(pipe.start), find(pipe.end)
        if ra != rb:
            parent[ra] = rb

    anchored = {find(node) for node in network.fixed_heads}
    orphans = sorted(
        node for node in network.nodes if find(node) not in anchored
    )
    if orphans:
        raise ModelError(
            "connected component without any fixed-head reference, "
            f"involving nodes: {', '.join(orphans)}"
        )


def load_scenarios(path: str | Path) -> list[Scenario]:
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(data, dict) or not isinstance(data.get("scenarios"), list):
        raise ModelError("scenario file must contain a 'scenarios' array")

    scenarios: list[Scenario] = []
    seen_names: set[str] = set()
    for index, item in enumerate(data["scenarios"]):
        if not isinstance(item, dict):
            raise ModelError(f"scenario #{index} must be an object")
        name = str(item.get("name", f"scenario-{index}"))
        if name in seen_names:
            raise ModelError(f"duplicate scenario name {name!r}")
        seen_names.add(name)
        raw_demands = item.get("demands", {})
        if not isinstance(raw_demands, dict):
            raise ModelError(f"scenario {name!r}: 'demands' must be an object")
        demands = {
            str(node): _require_number(value, f"demand of node {node!r} in scenario {name!r}")
            for node, value in raw_demands.items()
        }
        scenarios.append(Scenario(name=name, demands=demands))
    if not scenarios:
        raise ModelError("scenario file contains no scenarios")
    return scenarios
