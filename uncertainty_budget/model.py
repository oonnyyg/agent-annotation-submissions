"""Loading and validation of the JSON specification."""

from __future__ import annotations

import math

from .errors import InputSpecError

# Tolerances for matrix checks; see README for the rationale.
SYMMETRY_TOL = 1e-9
DIAGONAL_TOL = 1e-9
EIGEN_TOL = 1e-12


def _is_number(x) -> bool:
    return isinstance(x, (int, float)) and not isinstance(x, bool) and math.isfinite(x)


def load_inputs(spec: dict) -> list[dict]:
    raw = spec.get("inputs")
    if not isinstance(raw, list) or not raw:
        raise InputSpecError("'inputs' must be a non-empty list")
    inputs = []
    seen = set()
    for i, entry in enumerate(raw):
        if not isinstance(entry, dict):
            raise InputSpecError(f"inputs[{i}] must be an object")
        name = entry.get("name")
        if not isinstance(name, str) or not name.isidentifier():
            raise InputSpecError(f"inputs[{i}].name must be a valid identifier")
        if name in seen:
            raise InputSpecError(f"duplicate input name {name!r}")
        seen.add(name)
        value = entry.get("value")
        if not _is_number(value):
            raise InputSpecError(f"inputs[{i}].value must be a finite number")
        std = entry.get("std_uncertainty")
        if not _is_number(std) or std < 0.0:
            raise InputSpecError(
                f"inputs[{i}].std_uncertainty must be a non-negative finite number"
            )
        inputs.append({"name": name, "value": float(value), "std_uncertainty": float(std)})
    return inputs


def load_results(spec: dict, input_names: set[str]) -> list[dict]:
    raw = spec.get("results")
    if not isinstance(raw, list) or not raw:
        raise InputSpecError("'results' must be a non-empty list")
    results = []
    seen = set()
    for i, entry in enumerate(raw):
        if not isinstance(entry, dict):
            raise InputSpecError(f"results[{i}] must be an object")
        name = entry.get("name")
        if not isinstance(name, str) or not name:
            raise InputSpecError(f"results[{i}].name must be a non-empty string")
        if name in seen:
            raise InputSpecError(f"duplicate result name {name!r}")
        seen.add(name)
        expr = entry.get("expression")
        if not isinstance(expr, str):
            raise InputSpecError(f"results[{i}].expression must be a string")
        if name in input_names:
            raise InputSpecError(f"result name {name!r} collides with an input name")
        results.append({"name": name, "expression": expr})
    return results


def _jacobi_eigenvalues(matrix: list[list[float]]) -> list[float]:
    """Eigenvalues of a symmetric matrix via cyclic Jacobi rotations."""
    n = len(matrix)
    a = [row[:] for row in matrix]
    for _ in range(100 * max(1, n * n)):
        off = math.sqrt(sum(a[i][j] ** 2 for i in range(n) for j in range(n) if i != j))
        diag = math.sqrt(sum(a[i][i] ** 2 for i in range(n)))
        if off <= EIGEN_TOL * max(1.0, diag):
            break
        for p in range(n - 1):
            for q in range(p + 1, n):
                if a[p][q] == 0.0:
                    continue
                theta = (a[q][q] - a[p][p]) / (2.0 * a[p][q])
                t = math.copysign(1.0 / (abs(theta) + math.sqrt(theta * theta + 1.0)), theta)
                c = 1.0 / math.sqrt(t * t + 1.0)
                s = t * c
                for k in range(n):
                    akp, akq = a[k][p], a[k][q]
                    a[k][p] = c * akp - s * akq
                    a[k][q] = s * akp + c * akq
                for k in range(n):
                    apk, aqk = a[p][k], a[q][k]
                    a[p][k] = c * apk - s * aqk
                    a[q][k] = s * apk + c * aqk
    return [a[i][i] for i in range(n)]


def load_correlation(spec: dict, n: int) -> list[list[float]]:
    raw = spec.get("correlation")
    if raw is None:
        return [[1.0 if i == j else 0.0 for j in range(n)] for i in range(n)]
    if not isinstance(raw, list) or len(raw) != n:
        raise InputSpecError(f"'correlation' must be a {n}x{n} matrix")
    matrix = []
    for i, row in enumerate(raw):
        if not isinstance(row, list) or len(row) != n:
            raise InputSpecError(f"correlation row {i} must have {n} entries")
        for j, cell in enumerate(row):
            if not _is_number(cell):
                raise InputSpecError(f"correlation[{i}][{j}] must be a finite number")
        matrix.append([float(c) for c in row])
    for i in range(n):
        if abs(matrix[i][i] - 1.0) > DIAGONAL_TOL:
            raise InputSpecError(f"correlation diagonal entry [{i}][{i}] must be 1")
        for j in range(i + 1, n):
            if abs(matrix[i][j] - matrix[j][i]) > SYMMETRY_TOL:
                raise InputSpecError(f"correlation matrix is not symmetric at [{i}][{j}]")
    scale = max(1.0, max(abs(c) for row in matrix for c in row))
    for k, eig in enumerate(_jacobi_eigenvalues(matrix)):
        if eig < -EIGEN_TOL * scale * max(1, n):
            raise InputSpecError(
                "correlation matrix is not positive semidefinite "
                f"(eigenvalue {k} is {eig:.6g})"
            )
    return matrix
