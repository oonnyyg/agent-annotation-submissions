"""Input loading and structural validation."""

from __future__ import annotations

import csv
import json
from dataclasses import dataclass, field
from pathlib import Path

MAX_STATES = 6
UNKNOWN = "?"


class InputError(ValueError):
    """Raised when the tree, cost matrix, or observation data is invalid."""


@dataclass
class Node:
    node_id: str
    children: list["Node"] = field(default_factory=list)

    @property
    def is_leaf(self) -> bool:
        return not self.children


@dataclass
class Problem:
    root: Node
    states: list[str]
    costs: list[list[float]]
    characters: list[str]
    observations: dict[str, list[str | None]]

    def leaves(self) -> list[Node]:
        found: list[Node] = []

        def walk(node: Node) -> None:
            if node.is_leaf:
                found.append(node)
            for child in node.children:
                walk(child)

        walk(self.root)
        return found


def _parse_tree(raw: object) -> Node:
    if not isinstance(raw, dict):
        raise InputError("tree nodes must be JSON objects")
    node_id = raw.get("id")
    if not isinstance(node_id, (str, int)) or isinstance(node_id, bool):
        raise InputError("every tree node needs a string or integer 'id'")
    children_raw = raw.get("children", [])
    if not isinstance(children_raw, list):
        raise InputError(f"'children' of node {node_id!r} must be a list")
    node = Node(node_id=str(node_id))
    node.children = [_parse_tree(child) for child in children_raw]
    return node


def _check_tree(root: Node) -> None:
    seen: set[str] = set()

    def walk(node: Node, path: set[str]) -> None:
        if node.node_id in path:
            raise InputError(f"cycle detected at node {node.node_id!r}")
        if node.node_id in seen:
            raise InputError(f"node id {node.node_id!r} appears more than once")
        seen.add(node.node_id)
        path.add(node.node_id)
        for child in node.children:
            walk(child, path)
        path.discard(node.node_id)

    walk(root, set())


def _validate_costs(states: list[str], costs: object) -> list[list[float]]:
    n = len(states)
    if not (1 <= n <= MAX_STATES):
        raise InputError(f"state set must have 1..{MAX_STATES} states, got {n}")
    if len(set(states)) != n:
        raise InputError("state labels must be unique")
    if not isinstance(costs, list) or len(costs) != n:
        raise InputError("cost matrix must be a square list matching the states")
    matrix: list[list[float]] = []
    for i, row in enumerate(costs):
        if not isinstance(row, list) or len(row) != n:
            raise InputError(f"cost row {i} must have {n} entries")
        parsed = []
        for j, value in enumerate(row):
            if not isinstance(value, (int, float)) or isinstance(value, bool):
                raise InputError(f"cost[{i}][{j}] is not a number")
            if value < 0:
                raise InputError(f"cost[{i}][{j}] is negative")
            parsed.append(float(value))
        if parsed[i] != 0.0:
            raise InputError(f"diagonal cost[{i}][{i}] must be zero")
        matrix.append(parsed)
    return matrix


def _load_observations(
    path: Path, leaf_ids: set[str], states: list[str]
) -> tuple[list[str], dict[str, list[str | None]]]:
    with path.open(newline="", encoding="utf-8-sig") as handle:
        reader = csv.reader(handle)
        rows = [row for row in reader if row]
    if not rows:
        raise InputError("CSV file is empty")
    header = rows[0]
    if len(header) < 2:
        raise InputError("CSV header must be: id, then one column per character")
    characters = header[1:]
    width = len(header)
    observations: dict[str, list[str | None]] = {}
    for line, row in enumerate(rows[1:], start=2):
        if len(row) != width:
            raise InputError(f"CSV line {line} has {len(row)} fields, expected {width}")
        node_id = row[0].strip()
        if node_id in observations:
            raise InputError(f"duplicate CSV row for leaf {node_id!r}")
        values: list[str | None] = []
        for raw in row[1:]:
            value = raw.strip()
            if value == UNKNOWN:
                values.append(None)
            elif value in states:
                values.append(value)
            else:
                raise InputError(
                    f"CSV line {line}: state {value!r} is not in the state set"
                )
        observations[node_id] = values
    leaf_set = set(observations)
    missing_rows = leaf_ids - leaf_set
    extra_rows = leaf_set - leaf_ids
    if missing_rows:
        raise InputError(
            f"leaves without a CSV row (missing data is not 'unknown'): "
            f"{sorted(missing_rows)}"
        )
    if extra_rows:
        raise InputError(f"CSV rows without a matching leaf: {sorted(extra_rows)}")
    return characters, observations


def load_problem(tree_path: str | Path, csv_path: str | Path) -> Problem:
    """Load and validate the rooted tree, cost model, and leaf observations."""
    with open(tree_path, encoding="utf-8-sig") as handle:
        spec = json.load(handle)
    if not isinstance(spec, dict):
        raise InputError("tree JSON must be an object")
    states_raw = spec.get("states")
    if not isinstance(states_raw, list) or not all(
        isinstance(s, str) for s in states_raw
    ):
        raise InputError("'states' must be a list of state labels")
    costs = _validate_costs(states_raw, spec.get("costs"))
    tree_raw = spec.get("tree")
    if tree_raw is None:
        raise InputError("missing 'tree' object")
    root = _parse_tree(tree_raw)
    _check_tree(root)
    problem = Problem(
        root=root,
        states=list(states_raw),
        costs=costs,
        characters=[],
        observations={},
    )
    leaf_ids = {leaf.node_id for leaf in problem.leaves()}
    characters, observations = _load_observations(
        Path(csv_path), leaf_ids, problem.states
    )
    problem.characters = characters
    problem.observations = observations
    return problem
