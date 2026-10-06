"""Minimal dense linear algebra helpers (stdlib only)."""

from __future__ import annotations

import math


def solve_linear(matrix: list[list[float]], rhs: list[float]) -> list[float]:
    """Solve a square system by Gaussian elimination with partial pivoting."""
    n = len(matrix)
    aug = [row[:] + [rhs[i]] for i, row in enumerate(matrix)]
    for col in range(n):
        pivot = max(range(col, n), key=lambda r: abs(aug[r][col]))
        if abs(aug[pivot][col]) < 1e-14:
            raise ValueError("singular matrix")
        aug[col], aug[pivot] = aug[pivot], aug[col]
        for row in range(col + 1, n):
            factor = aug[row][col] / aug[col][col]
            for k in range(col, n + 1):
                aug[row][k] -= factor * aug[col][k]
    out = [0.0] * n
    for row in range(n - 1, -1, -1):
        acc = aug[row][n] - sum(aug[row][k] * out[k] for k in range(row + 1, n))
        out[row] = acc / aug[row][row]
    return out


def invert(matrix: list[list[float]]) -> list[list[float]]:
    """Invert a square matrix via Gauss-Jordan with partial pivoting."""
    n = len(matrix)
    aug = [row[:] + [1.0 if i == j else 0.0 for j in range(n)]
           for i, row in enumerate(matrix)]
    for col in range(n):
        pivot = max(range(col, n), key=lambda r: abs(aug[r][col]))
        if abs(aug[pivot][col]) < 1e-14:
            raise ValueError("singular matrix")
        aug[col], aug[pivot] = aug[pivot], aug[col]
        scale = aug[col][col]
        aug[col] = [v / scale for v in aug[col]]
        for row in range(n):
            if row != col and aug[row][col] != 0.0:
                factor = aug[row][col]
                aug[row] = [v - factor * p for v, p in zip(aug[row], aug[col])]
    return [row[n:] for row in aug]


def matrix_rank(matrix: list[list[float]], tol: float = 1e-10) -> int:
    """Numerical rank via row echelon reduction with scaled pivot threshold."""
    work = [row[:] for row in matrix]
    rows = len(work)
    cols = len(work[0]) if rows else 0
    scale = max((abs(v) for row in work for v in row), default=0.0)
    if scale == 0.0:
        return 0
    threshold = tol * scale * max(rows, cols)
    rank = 0
    col = 0
    for row in range(rows):
        while col < cols:
            pivot = max(range(row, rows), key=lambda r: abs(work[r][col]))
            if abs(work[pivot][col]) > threshold:
                break
            col += 1
        if col >= cols:
            break
        work[row], work[pivot] = work[pivot], work[row]
        for r in range(row + 1, rows):
            factor = work[r][col] / work[row][col]
            for k in range(col, cols):
                work[r][k] -= factor * work[row][k]
        rank += 1
        col += 1
    return rank


def is_finite_vector(values: list[float]) -> bool:
    return all(math.isfinite(v) for v in values)
