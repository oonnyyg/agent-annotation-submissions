"""Visit-ratio computation by solving the routing balance equations directly.

Solving the linear system (instead of power iteration) makes periodic
routing matrices work: no oscillating iterates, just the exact solution of
v = v P with v[reference] = 1.
"""
from __future__ import annotations

from .model import InputError, Network


def _solve_square(matrix: list[list[float]], rhs: list[float]) -> list[float]:
    """Gaussian elimination with partial pivoting (stdlib only)."""
    n = len(rhs)
    a = [row[:] + [rhs[i]] for i, row in enumerate(matrix)]
    for col in range(n):
        pivot = max(range(col, n), key=lambda r: abs(a[r][col]))
        if abs(a[pivot][col]) < 1e-12:
            raise InputError("routing: balance equations are singular; "
                             "the routing graph may not be a single closed chain")
        a[col], a[pivot] = a[pivot], a[col]
        for r in range(col + 1, n):
            factor = a[r][col] / a[col][col]
            if factor:
                for c in range(col, n + 1):
                    a[r][c] -= factor * a[col][c]
    x = [0.0] * n
    for r in range(n - 1, -1, -1):
        x[r] = (a[r][n] - sum(a[r][c] * x[c] for c in range(r + 1, n))) / a[r][r]
    return x


def visit_ratios(net: Network) -> dict[str, float]:
    """Return visit ratios v_i (visits to i per visit to the reference station).

    Solves v = v P, v[ref] = 1. For a strongly connected stochastic P the
    solution exists, is unique and is positive.
    """
    names = net.station_names()
    idx = {n: i for i, n in enumerate(names)}
    ref = idx[net.reference_station]
    n = len(names)
    others = [j for j in range(n) if j != ref]
    m = n - 1
    if m == 0:
        return {names[0]: 1.0}
    matrix = [[0.0] * m for _ in range(m)]
    rhs = [0.0] * m
    for row_pos, j in enumerate(others):
        # v_j - sum_i v_i P[i][j] = 0, substitute v_ref = 1.
        matrix[row_pos][row_pos] = 1.0
        for col_pos, i in enumerate(others):
            matrix[row_pos][col_pos] -= net.routing[names[i]].get(names[j], 0.0)
        rhs[row_pos] = net.routing[names[ref]].get(names[j], 0.0)
    sol = _solve_square(matrix, rhs)
    ratios = {names[ref]: 1.0}
    for col_pos, j in enumerate(others):
        v = sol[col_pos]
        if v <= 0:
            raise InputError(
                f"routing: computed visit ratio for {names[j]!r} is {v}; "
                "expected positive values in a strongly connected network")
        ratios[names[j]] = v
    return ratios
