"""Dense matrix helpers and a scaling-and-squaring matrix exponential.
 
Only the Python standard library is used. Matrices are lists of rows of
floats. The exponential uses Taylor series with scaling and squaring,
which handles repeated eigenvalues (equal decay constants) without any
division by rate differences.
"""
 
from __future__ import annotations
 
import math
 
 
class ConvergenceError(RuntimeError):
    """Raised when a numerical algorithm fails to converge."""
 
 
def matmul(a: list[list[float]], b: list[list[float]]) -> list[list[float]]:
    n = len(a)
    m = len(b[0])
    inner = len(b)
    result = [[0.0] * m for _ in range(n)]
    for i in range(n):
        ai = a[i]
        ri = result[i]
        for k in range(inner):
            aik = ai[k]
            if aik == 0.0:
                continue
            bk = b[k]
            for j in range(m):
                ri[j] += aik * bk[j]
    return result
 
 
def matvec(a: list[list[float]], x: list[float]) -> list[float]:
    return [sum(aij * xj for aij, xj in zip(row, x)) for row in a]
 
 
def identity(n: int) -> list[list[float]]:
    return [[1.0 if i == j else 0.0 for j in range(n)] for i in range(n)]
 
 
def _inf_norm(a: list[list[float]]) -> float:
    return max((sum(abs(v) for v in row) for row in a), default=0.0)
 
 
def matrix_exp(a: list[list[float]], tol: float = 1e-14,
               max_terms: int = 200) -> tuple[list[list[float]], dict]:
    """Compute exp(a) by scaling and squaring with a Taylor series.
 
    Returns (exp(a), info) where info reports the scaling exponent and
    the number of Taylor terms used, for precision auditing.
    """
    n = len(a)
    norm = _inf_norm(a)
    scale = 0
    if norm > 0.5:
        scale = max(1, math.ceil(math.log2(norm / 0.5)) + 1)
    factor = 2.0 ** -scale
    scaled = [[v * factor for v in row] for row in a]
 
    result = identity(n)
    term = identity(n)
    terms_used = 0
    for k in range(1, max_terms + 1):
        term = matmul(term, scaled)
        inv = 1.0 / k
        term = [[v * inv for v in row] for row in term]
        result = [[r + t for r, t in zip(rr, tr)]
                  for rr, tr in zip(result, term)]
        terms_used = k
        if _inf_norm(term) <= tol * max(1.0, _inf_norm(result)):
            break
    else:
        raise ConvergenceError(
            f"matrix exponential Taylor series did not converge in "
            f"{max_terms} terms (norm={norm!r})")
 
    for _ in range(scale):
        result = matmul(result, result)
 
    info = {"scaling_squarings": scale, "taylor_terms": terms_used,
            "tolerance": tol}
    return result, info
