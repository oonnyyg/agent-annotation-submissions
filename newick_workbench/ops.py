"""Distances, pruning, and midpoint rooting. All arithmetic is exact."""

from __future__ import annotations

from fractions import Fraction
from itertools import combinations

from .model import Node


def _depths(root):
    depths = {root: Fraction(0)}
    stack = [root]
    while stack:
        node = stack.pop()
        for child in node.children:
            depths[child] = depths[node] + child.length
            stack.append(child)
    return depths


def _lca(a, b):
    ancestors = set()
    while a is not None:
        ancestors.add(a)
        a = a.parent
    while b not in ancestors:
        b = b.parent
    return b


def _root_of(node):
    while node.parent is not None:
        node = node.parent
    return node


def distance(a, b):
    """Path distance between two nodes; shared ancestors counted once."""
    depths = _depths(_root_of(a))
    lca = _lca(a, b)
    return depths[a] + depths[b] - 2 * depths[lca]


def distance_matrix(root):
    """Return (names, matrix) with leaf names sorted; matrix is symmetric."""
    leaves = sorted(root.leaves(), key=lambda leaf: leaf.name)
    names = [leaf.name for leaf in leaves]
    depths = _depths(root)
    size = len(leaves)
    matrix = [[Fraction(0)] * size for _ in range(size)]
    for i, j in combinations(range(size), 2):
        lca = _lca(leaves[i], leaves[j])
        d = depths[leaves[i]] + depths[leaves[j]] - 2 * depths[lca]
        matrix[i][j] = matrix[j][i] = d
    return names, matrix


def prune(root, keep):
    """Keep only the named leaves. Returns (new_root, removed_internal_labels).

    Degree-2 non-root nodes are compressed by summing edge lengths; a root
    left with one child is replaced by that child (its edge is dropped).
    Removed internal labels are reported, never moved to other nodes.
    """
    keep_set = set(keep)
    if len(keep_set) < 2:
        raise ValueError("at least two leaf names are required to keep")
    leaf_names = {leaf.name for leaf in root.leaves()}
    missing = sorted(keep_set - leaf_names)
    if missing:
        raise ValueError(f"unknown leaf names: {', '.join(missing)}")
    removed = []

    def rebuild(node, is_root):
        if node.is_leaf:
            return node if node.name in keep_set else None
        kept_children = []
        for child in node.children:
            rebuilt = rebuild(child, False)
            if rebuilt is None:
                if not child.is_leaf and child.name:
                    removed.append(child.name)
            else:
                kept_children.append(rebuilt)
        node.children = kept_children
        for child in kept_children:
            child.parent = node
        if not kept_children:
            return None
        if is_root:
            return node
        if len(kept_children) == 1:
            child = kept_children[0]
            child.length = node.length + child.length
            if node.name:
                removed.append(node.name)
            return child
        return node

    new_root = rebuild(root, True)
    while len(new_root.children) == 1:
        child = new_root.children[0]
        if new_root.name:
            removed.append(new_root.name)
        child.length = None
        child.parent = None
        new_root = child
    return new_root, removed


def _path_between(a, b):
    """Node path from a to b (inclusive) using parent pointers."""
    anc_a = set()
    node = a
    while node is not None:
        anc_a.add(node)
        node = node.parent
    up_b = []
    node = b
    while node not in anc_a:
        up_b.append(node)
        node = node.parent
    meeting = node
    down_a = []
    node = a
    while node is not meeting:
        down_a.append(node)
        node = node.parent
    return down_a + [meeting] + list(reversed(up_b))


def midpoint_root(root):
    """Re-root the tree at its midpoint. Distances are preserved exactly.

    The farthest leaf pair wins; ties go to the lexicographically smallest
    sorted name pair. The halfway point is located walking from the
    lexicographically smaller leaf, so with consecutive zero-length edges
    the first node reached at exactly half distance becomes the root.
    A midpoint falling on a leaf attaches that leaf to a new root with a
    zero-length edge, keeping the leaf's identity.
    """
    leaves = sorted(root.leaves(), key=lambda leaf: leaf.name)
    if len(leaves) < 2:
        raise ValueError("midpoint rooting needs at least two leaves")
    depths = _depths(root)
    best_pair, best_dist = None, None
    for a, b in combinations(leaves, 2):
        lca = _lca(a, b)
        d = depths[a] + depths[b] - 2 * depths[lca]
        if best_dist is None or d > best_dist:
            best_pair, best_dist = (a, b), d
    start, target = best_pair
    half = best_dist / 2

    path = _path_between(start, target)
    cumulative = Fraction(0)
    position = ("node", start)
    if half > 0:
        for u, v in zip(path, path[1:]):
            edge = v.length if v.parent is u else u.length
            if cumulative + edge > half:
                position = ("edge", u, v, edge, half - cumulative)
                break
            cumulative += edge
            if cumulative == half:
                position = ("node", v)
                break

    adjacency = {}
    for node in root.iter_nodes():
        adjacency.setdefault(node, [])
        for child in node.children:
            adjacency.setdefault(child, [])
            adjacency[node].append((child, child.length))
            adjacency[child].append((node, child.length))

    kind = position[0]
    if kind == "node":
        pivot = position[1]
        if pivot.is_leaf:
            new_root = Node()
            neighbors = adjacency[pivot]
            adjacency[new_root] = [(pivot, Fraction(0))] + list(neighbors)
            for neighbor, weight in neighbors:
                adjacency[neighbor] = [
                    (n, w) for n, w in adjacency[neighbor] if n is not pivot
                ]
                adjacency[neighbor].append((new_root, weight))
            adjacency[pivot] = [(new_root, Fraction(0))]
        else:
            new_root = pivot
    else:
        _, u, v, edge, offset = position
        new_root = Node()
        adjacency[u] = [(n, w) for n, w in adjacency[u] if n is not v]
        adjacency[v] = [(n, w) for n, w in adjacency[v] if n is not u]
        adjacency[u].append((new_root, offset))
        adjacency[v].append((new_root, edge - offset))
        adjacency[new_root] = [(u, offset), (v, edge - offset)]

    def build(node, parent):
        built = Node(name=node.name)
        built.parent = parent
        for neighbor, weight in adjacency[node]:
            if neighbor is parent:
                continue
            child = build(neighbor, node)
            child.length = weight
            child.parent = built
            built.children.append(child)
        return built

    new_root.length = None
    return build(new_root, None)
