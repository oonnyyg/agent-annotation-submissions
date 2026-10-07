"""Quality-guided maximum-spanning-forest phase unwrapping."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .grid import GridData, TWO_PI, normalize_phase


class _UnionFind:
    def __init__(self, size: int) -> None:
        self.parent = list(range(size))
        self.rank = [0] * size

    def find(self, x: int) -> int:
        root = x
        while self.parent[root] != root:
            root = self.parent[root]
        while self.parent[x] != root:
            self.parent[x], x = root, self.parent[x]
        return root

    def union(self, a: int, b: int) -> bool:
        ra, rb = self.find(a), self.find(b)
        if ra == rb:
            return False
        if self.rank[ra] < self.rank[rb]:
            ra, rb = rb, ra
        self.parent[rb] = ra
        if self.rank[ra] == self.rank[rb]:
            self.rank[ra] += 1
        return True


@dataclass
class EdgeConflict:
    """Adjacency whose unwrapped difference differs from the principal
    phase difference by a non-zero integer number of cycles."""

    row_a: int
    col_a: int
    row_b: int
    col_b: int
    cycles: int
    is_tree_edge: bool
    same_region: bool


@dataclass
class UnwrapResult:
    rows: int
    cols: int
    unwrapped: list[list[float]]
    region: list[list[int]]
    cycles: list[list[int]]
    roots: list[tuple[int, int]]
    conflicts: list[EdgeConflict] = field(default_factory=list)


def _build_edges(grid: GridData) -> list[tuple[float, int, int]]:
    """All 4-neighbour edges as (weight, a, b) with a < b (flat indices)."""
    cols = grid.cols
    edges: list[tuple[float, int, int]] = []
    for r in range(grid.rows):
        for c in range(cols):
            if not grid.valid[r][c]:
                continue
            a = r * cols + c
            for dr, dc in ((0, 1), (1, 0)):
                nr, nc = r + dr, c + dc
                if nr < grid.rows and nc < cols and grid.valid[nr][nc]:
                    b = nr * cols + nc
                    w = min(grid.quality[r][c], grid.quality[nr][nc])
                    edges.append((w, a, b))
    return edges


def unwrap(grid: GridData) -> UnwrapResult:
    rows, cols = grid.rows, grid.cols
    size = rows * cols
    flat_phase = [grid.phase[r][c] for r in range(rows) for c in range(cols)]
    flat_valid = [grid.valid[r][c] for r in range(rows) for c in range(cols)]
    flat_quality = [grid.quality[r][c] for r in range(rows) for c in range(cols)]

    edges = _build_edges(grid)
    # Deterministic order: quality descending, ties by endpoint indices.
    edges.sort(key=lambda e: (-e[0], e[1], e[2]))

    uf = _UnionFind(size)
    tree_adj: list[list[tuple[int, float]]] = [[] for _ in range(size)]
    tree_edges: set[tuple[int, int]] = set()
    for w, a, b in edges:
        if uf.union(a, b):
            # Principal difference directed from smaller index a to b.
            d = normalize_phase(flat_phase[b] - flat_phase[a])
            tree_adj[a].append((b, d))
            tree_adj[b].append((a, -d))
            tree_edges.add((a, b))

    # Root of each component: highest quality, ties by smallest flat index.
    best: dict[int, int] = {}
    for i in range(size):
        if not flat_valid[i]:
            continue
        root_id = uf.find(i)
        cur = best.get(root_id)
        if cur is None or (flat_quality[i], -i) > (flat_quality[cur], -cur):
            best[root_id] = i

    # Deterministic region numbering ordered by root flat index.
    ordered_roots = sorted(best.items(), key=lambda kv: kv[1])
    region_of_root = {rid: n for n, (rid, _) in enumerate(ordered_roots)}

    flat_unwrapped = [math.nan] * size
    flat_region = [-1] * size
    for root_id, root in ordered_roots:
        region_id = region_of_root[root_id]
        flat_unwrapped[root] = flat_phase[root]
        flat_region[root] = region_id
        stack = [root]
        while stack:
            u = stack.pop()
            for v, delta in tree_adj[u]:
                if math.isnan(flat_unwrapped[v]):
                    flat_unwrapped[v] = flat_unwrapped[u] + delta
                    flat_region[v] = region_id
                    stack.append(v)

    # Integer cycle correspondence with the input phase.
    flat_cycles = [0] * size
    for i in range(size):
        if flat_valid[i]:
            flat_cycles[i] = round((flat_unwrapped[i] - flat_phase[i]) / TWO_PI)

    # Check every original adjacency edge, tree or not.
    conflicts: list[EdgeConflict] = []
    for w, a, b in edges:
        principal = normalize_phase(flat_phase[b] - flat_phase[a])
        actual = flat_unwrapped[b] - flat_unwrapped[a]
        off = round((actual - principal) / TWO_PI)
        if off != 0:
            ra, ca = divmod(a, cols)
            rb, cb = divmod(b, cols)
            conflicts.append(EdgeConflict(
                row_a=ra, col_a=ca, row_b=rb, col_b=cb,
                cycles=off,
                is_tree_edge=(a, b) in tree_edges,
                same_region=flat_region[a] == flat_region[b],
            ))

    unwrapped = [[flat_unwrapped[r * cols + c] for c in range(cols)]
                 for r in range(rows)]
    region = [[flat_region[r * cols + c] for c in range(cols)]
              for r in range(rows)]
    cycles = [[flat_cycles[r * cols + c] for c in range(cols)]
              for r in range(rows)]
    roots = [divmod(root, cols) for _, root in ordered_roots]

    return UnwrapResult(rows=rows, cols=cols, unwrapped=unwrapped,
                        region=region, cycles=cycles, roots=roots,
                        conflicts=conflicts)
