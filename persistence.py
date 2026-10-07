"""Zero-dimensional persistence for 2D point clouds (standard library only)."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

MAX_POINTS = 300
INFINITY = "Infinity"


@dataclass
class Merge:
    distance: float
    died: str
    survivor: str


@dataclass
class PersistenceResult:
    points: dict[str, tuple[float, float]]
    merges: list[Merge] = field(default_factory=list)
    edges: list[tuple[float, str, str]] = field(default_factory=list)

    def intervals(self) -> list[tuple[str, float, float | str]]:
        """(representative, birth, death) per component; death may be INFINITY."""
        deaths: dict[str, float] = {}
        for merge in self.merges:
            deaths[merge.died] = merge.distance
        out = []
        for pid in sorted(self.points):
            out.append((pid, 0.0, deaths.get(pid, INFINITY)))
        return out

    def components_at(self, threshold: float) -> list[list[str]]:
        """Groups after processing every merge with distance <= threshold."""
        if threshold < 0:
            raise ValueError("threshold must be non-negative")
        uf = _UnionFind(self.points.keys())
        for distance, a, b in self.edges:
            if distance <= threshold:
                uf.union(a, b)
        groups: dict[str, list[str]] = {}
        for pid in self.points:
            groups.setdefault(uf.find(pid), []).append(pid)
        return [sorted(members) for _, members in
                sorted(groups.items(), key=lambda kv: kv[0])]


class _UnionFind:
    """Root is always the lexicographically smallest member id."""

    def __init__(self, ids):
        self.parent = {pid: pid for pid in ids}

    def find(self, x: str) -> str:
        root = x
        while self.parent[root] != root:
            root = self.parent[root]
        while self.parent[x] != root:
            self.parent[x], x = root, self.parent[x]
        return root

    def union(self, a: str, b: str) -> tuple[str, str] | None:
        ra, rb = self.find(a), self.find(b)
        if ra == rb:
            return None
        survivor, died = (ra, rb) if ra < rb else (rb, ra)
        self.parent[died] = survivor
        return died, survivor


def compute_persistence(points: dict[str, tuple[float, float]]) -> PersistenceResult:
    result = PersistenceResult(points=dict(points))
    ids = sorted(points)
    dists: dict[float, list[tuple[str, str]]] = {}
    for i, a in enumerate(ids):
        ax, ay = points[a]
        for b in ids[i + 1:]:
            bx, by = points[b]
            d = math.hypot(ax - bx, ay - by)
            dists.setdefault(d, []).append((a, b))
    uf = _UnionFind(ids)
    for distance in sorted(dists):
        batch = sorted(dists[distance])
        for a, b in batch:
            result.edges.append((distance, a, b))
        died_roots: list[str] = []
        for a, b in batch:
            merged = uf.union(a, b)
            if merged is not None:
                died_roots.append(merged[0])
        # Records point at the representative after the whole batch is done.
        for died in died_roots:
            result.merges.append(Merge(distance, died, uf.find(died)))
    return result
