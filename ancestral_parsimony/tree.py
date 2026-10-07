"""Rooted tree parsing and structural validation."""

from __future__ import annotations

import json
from dataclasses import dataclass, field

from .errors import InputError


@dataclass
class Tree:
    """Rooted tree allowing multifurcation."""

    children: dict[int, list[int]]
    root: int
    leaves: list[int] = field(default_factory=list)

    def postorder(self) -> list[int]:
        order: list[int] = []
        stack = [(self.root, False)]
        while stack:
            node, expanded = stack.pop()
            if expanded:
                order.append(node)
            else:
                stack.append((node, True))
                for child in self.children[node]:
                    stack.append((child, False))
        return order

    def preorder_children_sorted(self) -> list[int]:
        """Preorder with children visited in sorted id order (deterministic)."""
        order: list[int] = []
        stack = [self.root]
        while stack:
            node = stack.pop()
            order.append(node)
            for child in sorted(self.children[node], reverse=True):
                stack.append(child)
        return order


def load_tree(path: str) -> Tree:
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, dict) or "nodes" not in data:
        raise InputError("tree JSON must be an object with a 'nodes' list")
    nodes = data["nodes"]
    if not isinstance(nodes, list) or not nodes:
        raise InputError("'nodes' must be a non-empty list")

    children: dict[int, list[int]] = {}
    parent_of: dict[int, int] = {}
    for entry in nodes:
        if not isinstance(entry, dict) or "id" not in entry:
            raise InputError("each node entry must be an object with an 'id'")
        node_id = entry["id"]
        if not isinstance(node_id, int) or isinstance(node_id, bool):
            raise InputError(f"node id must be an integer, got {node_id!r}")
        if node_id in children:
            raise InputError(f"duplicate node id {node_id}")
        kids = entry.get("children", [])
        if not isinstance(kids, list):
            raise InputError(f"children of node {node_id} must be a list")
        child_ids: list[int] = []
        for kid in kids:
            if not isinstance(kid, int) or isinstance(kid, bool):
                raise InputError(f"child id must be an integer, got {kid!r}")
            if kid in parent_of:
                raise InputError(
                    f"node {kid} has multiple parents ({parent_of[kid]} and {node_id})"
                )
            parent_of[kid] = node_id
            child_ids.append(kid)
        children[node_id] = child_ids

    for kids in children.values():
        for kid in kids:
            if kid not in children:
                raise InputError(f"child id {kid} referenced but not defined")

    roots = [n for n in children if n not in parent_of]
    if len(roots) != 1:
        raise InputError(
            f"tree must have exactly one root, found {len(roots)}: {roots}"
        )
    root = roots[0]

    # Reachability from the root also detects cycles (unreached nodes) and
    # disconnected components.
    seen: set[int] = set()
    stack = [root]
    while stack:
        node = stack.pop()
        if node in seen:
            raise InputError(f"cycle detected involving node {node}")
        seen.add(node)
        stack.extend(children[node])
    unreachable = set(children) - seen
    if unreachable:
        raise InputError(f"disconnected nodes not reachable from root: {sorted(unreachable)}")

    leaves = sorted(n for n in children if not children[n])
    return Tree(children=children, root=root, leaves=leaves)
