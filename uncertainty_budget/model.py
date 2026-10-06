"""Validation of input quantities and the correlation matrix."""

import math

from .errors import InputError

# Tolerance for symmetry / PSD checks, relative to matrix scale.
_TOL = 1e-9


class Quantity:
    def __init__(self, name, nominal, std_unc):
        self.name = name
        self.nominal = nominal
        self.std_unc = std_unc


def _require_number(value, what):
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise InputError(f"{what} must be a number, got {value!r}")
    if not math.isfinite(value):
        raise InputError(f"{what} must be finite, got {value!r}")
    return float(value)


def load_quantities(spec):
    """Validate the 'quantities' list; return list of Quantity."""
    if not isinstance(spec, list) or not spec:
        raise InputError("'quantities' must be a non-empty list")
    quantities = []
    seen = set()
    for entry in spec:
        if not isinstance(entry, dict):
            raise InputError(f"quantity entry must be an object, got {entry!r}")
        name = entry.get("name")
        if not isinstance(name, str) or not name.isidentifier():
            raise InputError(f"quantity name must be a valid identifier, got {name!r}")
        if name in seen:
            raise InputError(f"duplicate quantity name '{name}'")
        seen.add(name)
        nominal = _require_number(entry.get("nominal"), f"nominal of '{name}'")
        std_unc = _require_number(
            entry.get("std_uncertainty"), f"std_uncertainty of '{name}'"
        )
        if std_unc < 0.0:
            raise InputError(f"std_uncertainty of '{name}' must be non-negative")
        quantities.append(Quantity(name, nominal, std_unc))
    return quantities


def _check_psd(matrix, tol):
    """Check positive semidefiniteness via LDL^T (pivot) factorization.

    Accepts singular matrices (zero pivots are fine); rejects indefinite
    ones. Tolerance is relative to the diagonal scale.
    """
    n = len(matrix)
    scale = max([abs(matrix[i][i]) for i in range(n)] + [1.0])
    lower = [[0.0] * n for _ in range(n)]
    d = [0.0] * n
    for j in range(n):
        s = matrix[j][j]
        for k in range(j):
            s -= d[k] * lower[j][k] ** 2
        d[j] = s
        if abs(s) <= tol * scale:
            for i in range(j + 1, n):
                residual = matrix[i][j] - sum(
                    d[k] * lower[i][k] * lower[j][k] for k in range(j))
                if abs(residual) > tol * scale:
                    return False
        else:
            for i in range(j + 1, n):
                residual = matrix[i][j] - sum(
                    d[k] * lower[i][k] * lower[j][k] for k in range(j))
                lower[i][j] = residual / s
    return all(p >= -tol * scale for p in d)


def load_correlation(spec, n):
    """Validate the optional correlation matrix; return n x n list of floats."""
    if spec is None:
        return [[1.0 if i == j else 0.0 for j in range(n)] for i in range(n)]
    if not isinstance(spec, list) or len(spec) != n:
        raise InputError(f"correlation matrix must have {n} rows")
    matrix = []
    for i, row in enumerate(spec):
        if not isinstance(row, list) or len(row) != n:
            raise InputError(f"correlation row {i} must have {n} entries")
        matrix.append([
            _require_number(v, f"correlation[{i}][{j}]") for j, v in enumerate(row)
        ])
    for i in range(n):
        if abs(matrix[i][i] - 1.0) > _TOL:
            raise InputError(
                f"correlation diagonal [{i}][{i}] must be 1, got {matrix[i][i]}"
            )
    for i in range(n):
        for j in range(i + 1, n):
            if abs(matrix[i][j] - matrix[j][i]) > _TOL:
                raise InputError(
                    f"correlation matrix not symmetric at [{i}][{j}] vs [{j}][{i}]"
                )
    # Individual coefficients in [-1, 1] is necessary but NOT sufficient;
    # the whole matrix must also be positive semidefinite.
    for i in range(n):
        for j in range(n):
            if abs(matrix[i][j]) > 1.0 + _TOL:
                raise InputError(
                    f"correlation[{i}][{j}] = {matrix[i][j]} outside [-1, 1]"
                )
    if not _check_psd(matrix, _TOL):
        raise InputError(
            "correlation matrix is not positive semidefinite "
            "(coefficients individually in [-1, 1] is not sufficient)"
        )
    return matrix


def covariance_matrix(quantities, correlation):
    """Build the full input covariance matrix Sigma = D R D."""
    n = len(quantities)
    return [
        [
            quantities[i].std_unc * correlation[i][j] * quantities[j].std_unc
            for j in range(n)
        ]
        for i in range(n)
    ]
