"""Stiffness tensor handling: Voigt (engineering shear) conventions and validation."""

import math

from .errors import InputError

# Voigt index pairs in the required order 11,22,33,23,13,12.
VOIGT_PAIRS = ((0, 0), (1, 1), (2, 2), (1, 2), (0, 2), (0, 1))

_TOL = 1e-9


def _is_number(x):
    return isinstance(x, (int, float)) and not isinstance(x, bool) and math.isfinite(x)


def validate_density(value):
    if not _is_number(value) or value <= 0.0:
        raise InputError("density must be a positive finite number (kg/m^3)")
    return float(value)


def validate_stiffness(matrix):
    """Validate a 6x6 Voigt stiffness matrix (Pa, engineering shear strain).

    Must be symmetric and positive definite (checked via Cholesky).
    """
    if not isinstance(matrix, list) or len(matrix) != 6:
        raise InputError("stiffness must be a 6x6 matrix in Voigt order 11,22,33,23,13,12")
    c = []
    for row in matrix:
        if not isinstance(row, list) or len(row) != 6 or not all(_is_number(v) for v in row):
            raise InputError("stiffness rows must be 6 finite numbers")
        c.append([float(v) for v in row])
    scale = max(abs(c[i][j]) for i in range(6) for j in range(6))
    if scale == 0.0:
        raise InputError("stiffness matrix must be non-zero")
    for i in range(6):
        for j in range(i + 1, 6):
            if abs(c[i][j] - c[j][i]) > _TOL * scale:
                raise InputError("stiffness matrix must be symmetric")
    _cholesky(c)
    return c


def _cholesky(a, n=6):
    """Cholesky factorization; raises InputError unless `a` is positive definite."""
    lower = [[0.0] * n for _ in range(n)]
    for i in range(n):
        for j in range(i + 1):
            s = a[i][j] - sum(lower[i][k] * lower[j][k] for k in range(j))
            if i == j:
                if s <= 0.0:
                    raise InputError("stiffness matrix must be positive definite")
                lower[i][j] = math.sqrt(s)
            else:
                lower[i][j] = s / lower[j][j]
    return lower


def validate_orientation(matrix):
    """Validate a 3x3 orientation matrix.

    Columns are the crystal axes expressed in lab coordinates. Must be
    orthogonal with determinant +1 (a proper rotation).
    """
    if not isinstance(matrix, list) or len(matrix) != 3:
        raise InputError("orientation must be a 3x3 matrix")
    q = []
    for row in matrix:
        if not isinstance(row, list) or len(row) != 3 or not all(_is_number(v) for v in row):
            raise InputError("orientation rows must be 3 finite numbers")
        q.append([float(v) for v in row])
    for i in range(3):
        for j in range(3):
            expect = 1.0 if i == j else 0.0
            got = sum(q[k][i] * q[k][j] for k in range(3))
            if abs(got - expect) > 1e-6:
                raise InputError("orientation matrix must be orthogonal")
    det = (
        q[0][0] * (q[1][1] * q[2][2] - q[1][2] * q[2][1])
        - q[0][1] * (q[1][0] * q[2][2] - q[1][2] * q[2][0])
        + q[0][2] * (q[1][0] * q[2][1] - q[1][1] * q[2][0])
    )
    if abs(det - 1.0) > 1e-6:
        raise InputError("orientation matrix must have determinant +1 (proper rotation)")
    return q


def normalize_direction(vec):
    if not isinstance(vec, list) or len(vec) != 3 or not all(_is_number(v) for v in vec):
        raise InputError("each direction must be a list of 3 finite numbers")
    norm = math.sqrt(sum(float(v) ** 2 for v in vec))
    if norm == 0.0:
        raise InputError("propagation direction must be non-zero")
    return [float(v) / norm for v in vec]
