"""Min-cost flow solver for the transportation problem.

Uses successive shortest paths (SSP) with Dijkstra on a bipartite network.
Mass is augmented in whole bottleneck amounts per iteration, never expanded
into per-unit objects, so large masses stay cheap. All costs are
non-negative integers, so no potential reweighting is required.

Tie-breaking: Dijkstra pops nodes in (distance, node index) order and each
augmentation takes the bottleneck of the chosen path. Node indices follow
input order (sources before targets), so among equally optimal solutions
the solver deterministically prefers routes from earlier-listed sources to
earlier-listed targets. Identical inputs always yield identical plans.
"""
from __future__ import annotations

import heapq
from dataclasses import dataclass

from .model import Problem

INF = 1 << 62


@dataclass(frozen=True)
class Allocation:
    """Positive integer flow on one source -> target route."""

    source_id: str
    target_id: str
    amount: int
    unit_cost: int

    @property
    def cost(self) -> int:
        return self.amount * self.unit_cost


@dataclass(frozen=True)
class Solution:
    """Optimal transport plan with conservation guarantees."""

    allocations: tuple[Allocation, ...]
    total_cost: int
    total_mass: int

    @property
    def average_cost(self) -> float:
        return self.total_cost / self.total_mass


class InfeasibleError(Exception):
    """Raised when disabled routes make full transport impossible."""


class _Edge:
    __slots__ = ("to", "rev", "cap", "cost")

    def __init__(self, to: int, rev: int, cap: int, cost: int) -> None:
        self.to = to
        self.rev = rev
        self.cap = cap
        self.cost = cost


def _add_edge(graph: list[list[_Edge]], frm: int, to: int, cap: int, cost: int) -> None:
    graph[frm].append(_Edge(to, len(graph[to]), cap, cost))
    graph[to].append(_Edge(frm, len(graph[frm]) - 1, 0, -cost))


def solve(problem: Problem) -> Solution:
    """Return a globally optimal, deterministic transport plan.

    Raises InfeasibleError if disabled routes prevent moving all mass.
    """
    sources = problem.sources
    targets = problem.targets
    n, m = len(sources), len(targets)
    super_source = 0
    source_base = 1
    target_base = 1 + n
    sink = 1 + n + m
    size = sink + 1

    graph: list[list[_Edge]] = [[] for _ in range(size)]
    for i, site in enumerate(sources):
        _add_edge(graph, super_source, source_base + i, site.mass, 0)
    for j, site in enumerate(targets):
        _add_edge(graph, target_base + j, sink, site.mass, 0)
    disabled = problem.disabled
    for i, src in enumerate(sources):
        for j, dst in enumerate(targets):
            if (src.site_id, dst.site_id) in disabled:
                continue
            _add_edge(
                graph, source_base + i, target_base + j, INF, problem.unit_cost(src, dst)
            )

    total_mass = sum(s.mass for s in sources)
    sent = 0
    while sent < total_mass:
        dist = [INF] * size
        prev_node = [-1] * size
        prev_edge = [-1] * size
        dist[super_source] = 0
        heap: list[tuple[int, int]] = [(0, super_source)]
        while heap:
            d, node = heapq.heappop(heap)
            if d > dist[node]:
                continue
            for ei, edge in enumerate(graph[node]):
                if edge.cap <= 0:
                    continue
                nd = d + edge.cost
                if nd < dist[edge.to]:
                    dist[edge.to] = nd
                    prev_node[edge.to] = node
                    prev_edge[edge.to] = ei
                    heapq.heappush(heap, (nd, edge.to))
        if dist[sink] == INF:
            raise InfeasibleError(
                f"only {sent} of {total_mass} mass can be transported; "
                "disabled routes make the problem infeasible"
            )
        amount = total_mass - sent
        node = sink
        while node != super_source:
            edge = graph[prev_node[node]][prev_edge[node]]
            amount = min(amount, edge.cap)
            node = prev_node[node]
        node = sink
        while node != super_source:
            edge = graph[prev_node[node]][prev_edge[node]]
            edge.cap -= amount
            graph[node][edge.rev].cap += amount
            node = prev_node[node]
        sent += amount

    allocations: list[Allocation] = []
    for i, src in enumerate(sources):
        for edge in graph[source_base + i]:
            if not (target_base <= edge.to < target_base + m):
                continue
            used = graph[edge.to][edge.rev].cap
            if used > 0:
                dst = targets[edge.to - target_base]
                allocations.append(
                    Allocation(src.site_id, dst.site_id, used, edge.cost)
                )
    total_cost = sum(a.cost for a in allocations)
    return Solution(tuple(allocations), total_cost, total_mass)
