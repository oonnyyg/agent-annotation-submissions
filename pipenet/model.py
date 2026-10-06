"""Network and scenario data model with validation."""

from __future__ import annotations

import json
from dataclasses import dataclass, field


class ModelError(ValueError):
    """Raised when a network or scenario file is invalid."""


@dataclass(frozen=True)
class Node:
    node_id: str
    head: float | None  # fixed head boundary when not None
    demand: float  # positive = withdrawal, negative = injection


@dataclass(frozen=True)
class Pipe:
    pipe_id: str
    from_node: str
    to_node: str
    resistance: float  # head_loss = resistance * flow * |flow|


@dataclass
class Network:
    nodes: dict[str, Node]
    pipes: list[Pipe]
    fixed_ids: list[str] = field(default_factory=list)
    free_ids: list[str] = field(default_factory=list)


def _require(cond: bool, msg: str) -> None:
    if not cond:
        raise ModelError(msg)


def load_network(path: str) -> Network:
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    _require(isinstance(data, dict), "network file must be a JSON object")

    nodes: dict[str, Node] = {}
    for raw in data.get("nodes", []):
        nid = raw.get("id")
        _require(isinstance(nid, str) and nid, "every node needs a string id")
        _require(nid not in nodes, f"duplicate node id: {nid}")
        head = raw.get("head")
        demand = float(raw.get("demand", 0.0))
        if head is not None:
            head = float(head)
        nodes[nid] = Node(nid, head, demand)
    _require(nodes, "network has no nodes")

    pipes: list[Pipe] = []
    seen: set[str] = set()
    for raw in data.get("pipes", []):
        pid = raw.get("id")
        _require(isinstance(pid, str) and pid, "every pipe needs a string id")
        _require(pid not in seen, f"duplicate pipe id: {pid}")
        seen.add(pid)
        frm, to = raw.get("from"), raw.get("to")
        _require(frm in nodes, f"pipe {pid}: unknown from-node {frm!r}")
        _require(to in nodes, f"pipe {pid}: unknown to-node {to!r}")
        _require(frm != to, f"pipe {pid}: self-loops are not allowed")
        r = float(raw.get("resistance"))
        _require(r > 0.0, f"pipe {pid}: resistance must be positive")
        pipes.append(Pipe(pid, frm, to, r))
    _require(pipes, "network has no pipes")

    net = Network(nodes, pipes)
    net.fixed_ids = [n.node_id for n in nodes.values() if n.head is not None]
    net.free_ids = [n.node_id for n in nodes.values() if n.head is None]
    _require(net.fixed_ids, "network needs at least one fixed-head node")
    _check_components(net)
    return net


def _check_components(net: Network) -> None:
    adj: dict[str, list[str]] = {nid: [] for nid in net.nodes}
    for p in net.pipes:
        adj[p.from_node].append(p.to_node)
        adj[p.to_node].append(p.from_node)
    visited: set[str] = set()
    for start in net.nodes:
        if start in visited:
            continue
        stack, comp = [start], set()
        while stack:
            cur = stack.pop()
            if cur in comp:
                continue
            comp.add(cur)
            stack.extend(adj[cur])
        visited |= comp
        _require(
            any(nid in net.fixed_ids for nid in comp),
            "connected component without a fixed-head reference: "
            + ", ".join(sorted(comp)),
        )


def load_scenarios(path: str, net: Network) -> list[dict]:
    """Return a list of {'name': str, 'demands': {node_id: float}}."""
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    _require(isinstance(data, dict), "scenario file must be a JSON object")
    raw_list = data.get("scenarios")
    _require(isinstance(raw_list, list) and raw_list, "no scenarios defined")
    scenarios = []
    for i, raw in enumerate(raw_list):
        name = raw.get("name", f"scenario-{i}")
        demands = {nid: n.demand for nid, n in net.nodes.items()}
        for nid, val in raw.get("demands", {}).items():
            _require(nid in net.nodes, f"scenario {name}: unknown node {nid!r}")
            _require(
                net.nodes[nid].head is None,
                f"scenario {name}: cannot set demand on fixed-head node {nid!r}",
            )
            demands[nid] = float(val)
        scenarios.append({"name": name, "demands": demands})
    return scenarios
