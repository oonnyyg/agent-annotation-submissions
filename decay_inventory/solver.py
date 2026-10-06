"""Time integration of the linear decay system with external feeds.
 
The system dN/dt = A N + f(t) is integrated exactly (up to roundoff) on
each interval where the feed vector f is constant, using the matrix
exponential of an augmented system that also integrates the decay
counters J_i = integral of lambda_i N_i dt. This avoids dividing by
differences of decay constants, so equal or nearly equal half-lives are
handled naturally.
"""
 
from __future__ import annotations
 
from dataclasses import dataclass, field
 
from .linalg import ConvergenceError, matrix_exp, matvec
from .model import Network
 
TOL = 1e-14
NEGATIVE_TOL = 1e-9  # relative tolerance for the negative-quantity check
 
 
class SolverError(RuntimeError):
    """Raised when the computation fails or produces invalid results."""
 
 
@dataclass
class WindowResult:
    start: float
    end: float
    decays: list[float]            # per nuclide
    branch_yields: list[list[float]]  # branch_yields[parent][child_index]
 
 
@dataclass
class Solution:
    times: list[float]
    quantities: list[list[float]]   # quantities[k][i] at times[k]
    activities: list[list[float]]
    windows: list[WindowResult]
    conservation_residual: list[float]  # per output time
    cumulative_feed: list[float]        # per output time
    precision: dict = field(default_factory=dict)
 
 
def _build_system_matrix(net: Network) -> list[list[float]]:
    n = net.size
    a = [[0.0] * n for _ in range(n)]
    for i, nuclide in enumerate(net.nuclides):
        lam = nuclide.decay_constant
        if lam == 0.0:
            continue
        a[i][i] -= lam
        for child, prob in net.branches[i]:
            a[child][i] += lam * prob
    return a
 
 
def _feed_vector(net: Network, t: float) -> list[float]:
    f = [0.0] * net.size
    for feed in net.feeds:
        if feed.start <= t < feed.end:
            f[feed.target] += feed.rate
    return f
 
 
def _augmented_matrix(a: list[list[float]], f: list[float],
                      lambdas: list[float]) -> list[list[float]]:
    n = len(a)
    size = 2 * n + 1
    m = [[0.0] * size for _ in range(size)]
    for i in range(n):
        for j in range(n):
            m[i][j] = a[i][j]
        m[i][n] = f[i]
        m[n + 1 + i][i] = lambdas[i]
    return m
 
 
def solve(net: Network, times: list[float],
          windows: list[tuple[float, float]]) -> Solution:
    if not times:
        raise SolverError("at least one output time is required")
    if any(t2 < t1 for t1, t2 in zip(times, times[1:])):
        raise SolverError("output times must be sorted in ascending order")
    for ws, we in windows:
        if we < ws:
            raise SolverError(f"window end {we} is before start {ws}")
 
    n = net.size
    a = _build_system_matrix(net)
    lambdas = [nuc.decay_constant for nuc in net.nuclides]
 
    breakpoints = {times[0]}
    breakpoints.update(times)
    for feed in net.feeds:
        breakpoints.add(feed.start)
        breakpoints.add(feed.end)
    for ws, we in windows:
        breakpoints.add(ws)
        breakpoints.add(we)
    ordered = sorted(breakpoints)
 
    n0 = [nuc.initial for nuc in net.nuclides]
    state = n0 + [1.0] + [0.0] * n  # [N, u, J]
 
    snap_j: dict[float, list[float]] = {}
    snap_n: dict[float, list[float]] = {}
    snap_feed: dict[float, float] = {}
    cumulative_feed = 0.0
    precision = {"matrix_exponential": [], "tolerance": TOL,
                 "negative_tolerance": NEGATIVE_TOL}
 
    t_prev = ordered[0]
    snap_j[t_prev] = state[n + 1:]
    snap_n[t_prev] = state[:n]
    snap_feed[t_prev] = 0.0
 
    for t_cur in ordered[1:]:
        dt = t_cur - t_prev
        if dt > 0.0:
            f = _feed_vector(net, t_prev)
            m = _augmented_matrix(a, f, lambdas)
            m = [[v * dt for v in row] for row in m]
            try:
                exp_m, info = matrix_exp(m, tol=TOL)
            except ConvergenceError as exc:
                raise SolverError(
                    f"failed to integrate interval [{t_prev}, {t_cur}]: {exc}"
                ) from exc
            precision["matrix_exponential"].append(
                {"interval": [t_prev, t_cur], **info})
            state = matvec(exp_m, state)
            cumulative_feed += sum(f) * dt
        t_prev = t_cur
        snap_j[t_cur] = state[n + 1:]
        snap_n[t_cur] = state[:n]
        snap_feed[t_cur] = cumulative_feed
 
    initial_total = sum(n0)
    out_quantities: list[list[float]] = []
    out_activities: list[list[float]] = []
    residuals: list[float] = []
    feeds_at_time: list[float] = []
    for t in times:
        quantities = list(snap_n[t])
        scale = max(1.0, max(abs(q) for q in quantities), initial_total)
        for i, q in enumerate(quantities):
            if q < -NEGATIVE_TOL * scale:
                raise SolverError(
                    f"computed quantity of {net.nuclides[i].name!r} at t={t} "
                    f"is negative ({q!r}); refusing to truncate to zero")
        out_quantities.append(quantities)
        out_activities.append([lambdas[i] * q for i, q in enumerate(quantities)])
        residuals.append(sum(quantities) - initial_total - snap_feed[t])
        feeds_at_time.append(snap_feed[t])
 
    window_results: list[WindowResult] = []
    for ws, we in windows:
        decays = [je - js for js, je in zip(snap_j[ws], snap_j[we])]
        yields: list[list[float]] = []
        for i in range(n):
            yields.append([decays[i] * prob
                           for _, prob in _children_of(net, i)])
        window_results.append(WindowResult(ws, we, decays, yields))
 
    return Solution(times, out_quantities, out_activities, window_results,
                    residuals, feeds_at_time, precision)
 
 
def _children_of(net: Network, parent: int) -> list[tuple[int, float]]:
    return list(net.branches[parent])
