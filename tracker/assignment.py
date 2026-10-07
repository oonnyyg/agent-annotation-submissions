"""Global one-to-one minimum-cost assignment (Hungarian algorithm).

Solves a rectangular assignment problem with optional "forbidden"
edges (infinite cost). Ties are broken deterministically by preferring
smaller row index, then smaller column index.
"""

from __future__ import annotations

import math
from typing import Sequence

INF = math.inf


def min_cost_assignment(cost: Sequence[Sequence[float]]) -> list[int]:
    """Return for each row the assigned column index, or -1 if unmatched.

    Rows with no feasible column are left unmatched; extra columns are
    ignored. Deterministic: among equal-cost solutions the
    lexicographically smallest assignment vector is returned.
    """
    n_rows = len(cost)
    n_cols = max((len(row) for row in cost), default=0)
    if n_rows == 0 or n_cols == 0:
        return [-1] * n_rows

    # Pad to a square matrix of size n = n_rows + n_cols so that
    # "leave unmatched" is modelled by dummy columns/rows of cost 0.
    # Forbidden real edges keep a huge finite cost. Real rows matched
    # to dummy columns mean "unmatched"; dummy rows absorb unused
    # real columns.
    n = n_rows + n_cols
    big = 1e18  # finite stand-in for forbidden edges
    shift = 1e12  # makes any feasible match cheaper than staying unmatched
    matrix = [[0.0] * n for _ in range(n)]
    for i in range(n_rows):
        for j in range(n_cols):
            c = cost[i][j]
            matrix[i][j] = big if math.isinf(c) else c - shift
        # dummy columns j >= n_cols stay 0 -> cost of leaving row unmatched
    # dummy rows i >= n_rows: all zeros

    assignment = hungarian_square(matrix)
    return [
        col if 0 <= col < n_cols and matrix[i][col] < big else -1
        for i, col in enumerate(assignment[:n_rows])
    ]


def hungarian_square(matrix: Sequence[Sequence[float]]) -> list[int]:
    """Solve a square assignment problem; return row -> column map.

    Deterministic: equal-cost options are resolved in favour of the
    lowest row index, then the lowest column index.
    """
    n = len(matrix)
    u = [0.0] * (n + 1)
    v = [0.0] * (n + 1)
    p = [0] * (n + 1)  # p[j] = row matched to column j
    way = [0] * (n + 1)
    for i in range(1, n + 1):
        p[0] = i
        j0 = 0
        minv = [INF] * (n + 1)
        used = [False] * (n + 1)
        while True:
            used[j0] = True
            i0 = p[j0]
            j1 = -1
            delta = INF
            for j in range(1, n + 1):
                if used[j]:
                    continue
                cur = matrix[i0 - 1][j - 1] - u[i0] - v[j]
                if cur < minv[j] - 1e-12:
                    minv[j] = cur
                    way[j] = j0
                if minv[j] < delta - 1e-12:
                    delta = minv[j]
                    j1 = j
            for j in range(0, n + 1):
                if used[j]:
                    u[p[j]] += delta
                    v[j] -= delta
                else:
                    minv[j] -= delta
            j0 = j1
            if p[j0] == 0:
                break
        while j0:
            j1 = way[j0]
            p[j0] = p[j1]
            j0 = j1

    assignment = [-1] * n
    for j in range(1, n + 1):
        assignment[p[j] - 1] = j - 1
    return assignment
