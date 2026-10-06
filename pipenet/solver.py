"""Damped Newton solver for steady-state heads and pipe flows.

Unknowns: heads at free nodes and flow in every pipe (positive along the
pipe's declared direction). Equations:
  node balance : inflow - outflow - demand = 0        (free nodes)
  pipe relation: H_from - H_to - r * q * |q| = 0      (all pipes)
Convergence requires both residual families below their tolerances.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .model import Network

_DERIV_FLOOR = 1e-12  # keeps the Jacobian nonsingular at q == 0


@dataclass
class SolveResult:
    converged: bool
    reason: str
    iterations: int
    node_residual: float
    pipe_residual: float
    heads: dict[str, float] = field(default_factory=dict)
    flows: dict[str, float] = field(default_factory=dict)


def _solve_dense(a: list[list[float]], b: list[float]) -> list[float] | None:
    """Gaussian elimination with partial pivoting; None if singular."""
    n = len(b)
    for col in range(n):
        piv = max(range(col, n), key=lambda r: abs(a[r][col]))
        if abs(a[piv][col]) < 1e-14:
            return None
        a[col], a[piv] = a[piv], a[col]
        b[col], b[piv] = b[piv], b[col]
        for row in range(col + 1, n):
            f = a[row][col] / a[col][col]
            if f == 0.0:
                continue
            for k in range(col, n):
                a[row][k] -= f * a[col][k]
            b[row] -= f * b[col]
    x = [0.0] * n
    for row in range(n - 1, -1, -1):
        s = b[row] - sum(a[row][k] * x[k] for k in range(row + 1, n))
        x[row] = s / a[row][row]
    return x


def solve(
    net: Network,
    demands: dict[str, float],
    max_iter: int = 50,
    tol_node: float = 1e-8,
    tol_pipe: float = 1e-8,
) -> SolveResult:
    free = net.free_ids
    hidx = {nid: i for i, nid in enumerate(free)}
    n_h, n_q = len(free), len(net.pipes)
    size = n_h + n_q

    head_of = {nid: net.nodes[nid].head for nid in net.fixed_ids}
    x = [head_of.get(nid, None) for nid in free]  # placeholder
    # initial heads: average of fixed heads; flows: zero
    avg = sum(head_of.values()) / len(head_of)
    x = [avg] * n_h + [0.0] * n_q

    def residuals(xv: list[float]) -> tuple[list[float], float, float]:
        res = [0.0] * size
        max_node = max_pipe = 0.0
        for i, nid in enumerate(free):
            bal = -demands[nid]
            for j, p in enumerate(net.pipes):
                q = xv[n_h + j]
                if p.to_node == nid:
                    bal += q
                elif p.from_node == nid:
                    bal -= q
            res[i] = bal
            max_node = max(max_node, abs(bal))
        for j, p in enumerate(net.pipes):
            hf = xv[hidx[p.from_node]] if p.from_node in hidx else head_of[p.from_node]
            ht = xv[hidx[p.to_node]] if p.to_node in hidx else head_of[p.to_node]
            q = xv[n_h + j]
            r = hf - ht - p.resistance * q * abs(q)
            res[n_h + j] = r
            max_pipe = max(max_pipe, abs(r))
        return res, max_node, max_pipe

    def jacobian(xv: list[float]) -> list[list[float]]:
        jac = [[0.0] * size for _ in range(size)]
        for i, nid in enumerate(free):
            for j, p in enumerate(net.pipes):
                if p.to_node == nid:
                    jac[i][n_h + j] = 1.0
                elif p.from_node == nid:
                    jac[i][n_h + j] = -1.0
        for j, p in enumerate(net.pipes):
            row = n_h + j
            if p.from_node in hidx:
                jac[row][hidx[p.from_node]] = 1.0
            if p.to_node in hidx:
                jac[row][hidx[p.to_node]] = -1.0
            q = xv[n_h + j]
            jac[row][row] = -max(2.0 * p.resistance * abs(q), _DERIV_FLOOR)
        return jac

    res, rn, rp = residuals(x)
    reason = "iteration limit reached"
    for it in range(1, max_iter + 1):
        if rn <= tol_node and rp <= tol_pipe:
            return _finish(net, hidx, x, True, "converged", it - 1, rn, rp)
        jac = jacobian(x)
        step = _solve_dense(jac, [-v for v in res])
        if step is None:
            reason = "singular Jacobian (numerical failure)"
            break
        if any(math.isnan(v) or math.isinf(v) for v in step):
            reason = "non-finite Newton step (numerical failure)"
            break
        # damped step: halve until the total residual norm decreases
        norm0 = math.sqrt(sum(v * v for v in res))
        lam = 1.0
        accepted = False
        for _ in range(30):
            cand = [a + lam * b for a, b in zip(x, step)]
            res_c, rn_c, rp_c = residuals(cand)
            norm_c = math.sqrt(sum(v * v for v in res_c))
            if norm_c < norm0 or norm_c == 0.0:
                x, res, rn, rp = cand, res_c, rn_c, rp_c
                accepted = True
                break
            lam *= 0.5
        if not accepted:
            reason = "line search failed to reduce residual"
            break
    return _finish(net, hidx, x, False, reason, max_iter, rn, rp)


def _finish(net, hidx, x, ok, reason, iters, rn, rp) -> SolveResult:
    n_h = len(net.free_ids)
    heads = {nid: net.nodes[nid].head for nid in net.fixed_ids}
    flows = {}
    if ok:
        for nid, i in hidx.items():
            heads[nid] = x[i]
        for j, p in enumerate(net.pipes):
            flows[p.pipe_id] = x[n_h + j]
    return SolveResult(ok, reason, iters, rn, rp, heads, flows)
