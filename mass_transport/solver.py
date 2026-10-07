"""Minimum-cost transportation solver (pure stdlib, integer flows).

Modelled as a min-cost flow problem on a bipartite network and solved with
successive shortest paths using Johnson potentials. Masses stay as integer
arc capacities, so large quantities are moved in bulk, never unit by unit.
All scans iterate nodes and arcs in sorted-id order, which makes the result
deterministic: among equally optimal plans the solver prefers the one that
is lexicographically smallest by (source id, target id) path order.
"""

from __future__ import annotations

import heapq
from dataclasses import dataclass

from .model import Problem

INF = float("inf")


@dataclass(frozen=True)
class FlowEntry:
    source: str
    target: str
    amount: int
    unit_cost: int

    @property
    def cost(self) -> int:
        return self.amount * self.unit_cost


@dataclass(frozen=True)
class SolveResult:
    feasible: bool
    flows: tuple[FlowEntry, ...]
    total_cost: int
    total_mass: int

    @property
    def average_cost(self) -> float:
        return self.total_cost / self.total_mass


def unit_cost(metric: str, dx: int, dy: int) -> int:
    if metric == "manhattan":
        return abs(dx) + abs(dy)
    if metric == "sqeuclidean":
        return dx * dx + dy * dy
    raise ValueError(f"unknown metric {metric!r}")


class _Edge:
    __slots__ = ("to", "rev", "cap", "cost")

    def __init__(self, to: int, rev: int, cap: int, cost: int) -> None:
        self.to = to
        self.rev = rev
        self.cap = cap
        self.cost = cost


class _MinCostFlow:
    def __init__(self, n: int) -> None:
        self.graph: list[list[_Edge]] = [[] for _ in range(n)]

    def add_edge(self, frm: int, to: int, cap: int, cost: int) -> None:
        fwd = _Edge(to, len(self.graph[to]), cap, cost)
        rev = _Edge(frm, len(self.graph[frm]), 0, -cost)
        self.graph[frm].append(fwd)
        self.graph[to].append(rev)

    def flow(self, s: int, t: int, limit: int) -> tuple[int, int]:
        """Send up to `limit` units s->t at min cost. Returns (flow, cost)."""
        n = len(self.graph)
        potential = [0] * n
        total_flow = 0
        total_cost = 0
        while total_flow < limit:
            dist = [INF] * n
            prev_node = [-1] * n
            prev_edge = [-1] * n
            dist[s] = 0
            heap = [(0, s)]
            while heap:
                d, v = heapq.heappop(heap)
                if d > dist[v]:
                    continue
                for i, e in enumerate(self.graph[v]):
                    if e.cap <= 0:
                        continue
                    nd = d + e.cost + potential[v] - potential[e.to]
                    if nd < dist[e.to]:
                        dist[e.to] = nd
                        prev_node[e.to] = v
                        prev_edge[e.to] = i
                        heapq.heappush(heap, (nd, e.to))
            if dist[t] == INF:
                break
            for v in range(n):
                if dist[v] < INF:
                    potential[v] += dist[v]
            add = limit - total_flow
            v = t
            while v != s:
                e = self.graph[prev_node[v]][prev_edge[v]]
                add = min(add, e.cap)
                v = prev_node[v]
            v = t
            while v != s:
                e = self.graph[prev_node[v]][prev_edge[v]]
                e.cap -= add
                self.graph[v][e.rev].cap += add
                v = prev_node[v]
            total_flow += add
            total_cost += add * potential[t]
        return total_flow, total_cost


def solve(problem: Problem) -> SolveResult:
    """Solve the transport problem; returns feasible=False if unsatisfiable."""
    sources = sorted(problem.sources.locations, key=lambda loc: loc.ident)
    targets = sorted(problem.targets.locations, key=lambda loc: loc.ident)
    total_mass = problem.sources.total_mass

    n_src = len(sources)
    n_dst = len(targets)
    s_node = n_src + n_dst
    t_node = s_node + 1
    mcf = _MinCostFlow(t_node + 1)

    for i, loc in enumerate(sources):
        mcf.add_edge(s_node, i, loc.mass, 0)
    arc_index: list[tuple[int, int]] = []  # (source idx, edge idx into graph)
    for i, sloc in enumerate(sources):
        for j, tloc in enumerate(targets):
            if (sloc.ident, tloc.ident) in problem.disabled:
                continue
            cost = unit_cost(
                problem.metric, sloc.x - tloc.x, sloc.y - tloc.y
            )
            arc_index.append((i, len(mcf.graph[i])))
            mcf.add_edge(i, n_src + j, total_mass, cost)
    for j, loc in enumerate(targets):
        mcf.add_edge(n_src + j, t_node, loc.mass, 0)

    sent, cost = mcf.flow(s_node, t_node, total_mass)
    if sent < total_mass:
        return SolveResult(False, (), 0, total_mass)

    flows: list[FlowEntry] = []
    for i, edge_idx in arc_index:
        edge = mcf.graph[i][edge_idx]
        used = mcf.graph[edge.to][edge.rev].cap
        if used > 0:
            j = edge.to - n_src
            flows.append(
                FlowEntry(sources[i].ident, targets[j].ident, used, edge.cost)
            )
    flows.sort(key=lambda f: (f.source, f.target))
    return SolveResult(True, tuple(flows), cost, total_mass)
