"""Core elastic-wave mathematics.

Conventions
-----------
* Stiffness is given in Pa as a symmetric positive-definite 6x6 Voigt
  matrix with engineering shear strains, order 11, 22, 33, 23, 13, 12.
* The orientation matrix Q has the crystal axes (expressed in lab
  coordinates) as its columns, so v_lab = Q @ v_crystal. Q must be
  orthogonal with determinant +1 (a proper rotation).
* The Christoffel matrix is Gamma_ik = C_ijkl n_j n_l, built from the
  full elastic tensor in lab coordinates -- never from diagonal Voigt
  entries alone.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

VOIGT_PAIRS = ((0, 0), (1, 1), (2, 2), (1, 2), (0, 2), (0, 1))


class InputError(ValueError):
    """Raised when the user-supplied configuration is invalid."""


def _is_number(x):
    return isinstance(x, (int, float)) and not isinstance(x, bool) and math.isfinite(x)


def validate_stiffness(matrix):
    """Return the 6x6 stiffness as floats, enforcing symmetry and SPD."""
    if not isinstance(matrix, list) or len(matrix) != 6:
        raise InputError("stiffness must be a 6x6 matrix")
    c = []
    for row in matrix:
        if not isinstance(row, list) or len(row) != 6:
            raise InputError("stiffness must be a 6x6 matrix")
        if not all(_is_number(v) for v in row):
            raise InputError("stiffness entries must be finite numbers")
        c.append([float(v) for v in row])
    for i in range(6):
        for j in range(i + 1, 6):
            if abs(c[i][j] - c[j][i]) > 1e-8 * max(1.0, abs(c[i][i]), abs(c[j][j])):
                raise InputError("stiffness matrix must be symmetric")
    _require_spd(c)
    return c


def _require_spd(m):
    """Cholesky-based symmetric positive-definite check (raises)."""
    n = len(m)
    lower = [[0.0] * n for _ in range(n)]
    for i in range(n):
        for j in range(i + 1):
            s = m[i][j] - sum(lower[i][k] * lower[j][k] for k in range(j))
            if i == j:
                if s <= 0.0:
                    raise InputError("stiffness matrix must be positive definite")
                lower[i][j] = math.sqrt(s)
            else:
                lower[i][j] = s / lower[j][j]


def validate_density(value):
    if not _is_number(value) or value <= 0.0:
        raise InputError("density must be a positive finite number")
    return float(value)


def validate_orientation(matrix):
    """Return Q (columns = crystal axes in lab frame) as floats."""
    if matrix is None:
        return [[1.0, 0.0, 0.0], [0.0, 1.0, 0.0], [0.0, 0.0, 1.0]]
    if not isinstance(matrix, list) or len(matrix) != 3:
        raise InputError("orientation must be a 3x3 matrix")
    q = []
    for row in matrix:
        if not isinstance(row, list) or len(row) != 3:
            raise InputError("orientation must be a 3x3 matrix")
        if not all(_is_number(v) for v in row):
            raise InputError("orientation entries must be finite numbers")
        q.append([float(v) for v in row])
    cols = [[q[i][j] for i in range(3)] for j in range(3)]
    for a in range(3):
        for b in range(a, 3):
            dot = sum(cols[a][i] * cols[b][i] for i in range(3))
            expect = 1.0 if a == b else 0.0
            if abs(dot - expect) > 1e-9:
                raise InputError("orientation columns must be orthonormal")
    det = (
        q[0][0] * (q[1][1] * q[2][2] - q[1][2] * q[2][1])
        - q[0][1] * (q[1][0] * q[2][2] - q[1][2] * q[2][0])
        + q[0][2] * (q[1][0] * q[2][1] - q[1][1] * q[2][0])
    )
    if abs(det - 1.0) > 1e-9:
        raise InputError("orientation determinant must be +1")
    return q


def validate_direction(vec):
    if not isinstance(vec, list) or len(vec) != 3 or not all(_is_number(v) for v in vec):
        raise InputError("each direction must be a list of 3 finite numbers")
    norm = math.sqrt(sum(v * v for v in vec))
    if norm == 0.0:
        raise InputError("propagation direction must be non-zero")
    return [float(v) / norm for v in vec]


def voigt_to_tensor(c):
    """Expand a 6x6 Voigt matrix to the full 3x3x3x3 elastic tensor."""
    t = [[[[0.0] * 3 for _ in range(3)] for _ in range(3)] for _ in range(3)]
    for a, (i, j) in enumerate(VOIGT_PAIRS):
        for b, (k, l) in enumerate(VOIGT_PAIRS):
            v = c[a][b]
            for p, q in ((i, j), (j, i)):
                for r, s in ((k, l), (l, k)):
                    t[p][q][r][s] = v
    return t


def tensor_to_voigt(t):
    c = [[0.0] * 6 for _ in range(6)]
    for a, (i, j) in enumerate(VOIGT_PAIRS):
        for b, (k, l) in enumerate(VOIGT_PAIRS):
            c[a][b] = t[i][j][k][l]
    return c


def rotate_tensor(t, q):
    """Rotate a 3x3x3x3 tensor: T'_ijkl = Q_im Q_jn Q_kp Q_lq T_mnpq."""
    out = [[[[0.0] * 3 for _ in range(3)] for _ in range(3)] for _ in range(3)]
    for i in range(3):
        for j in range(3):
            for k in range(3):
                for l in range(3):
                    s = 0.0
                    for m in range(3):
                        qim = q[i][m]
                        if qim == 0.0:
                            continue
                        for n in range(3):
                            qjn = q[j][n]
                            if qjn == 0.0:
                                continue
                            for p in range(3):
                                qkp = q[k][p]
                                if qkp == 0.0:
                                    continue
                                for r in range(3):
                                    s += qim * qjn * qkp * q[l][r] * t[m][n][p][r]
                    out[i][j][k][l] = s
    return out


def christoffel(t, n):
    """Gamma_ik = C_ijkl n_j n_l for unit direction n."""
    g = [[0.0] * 3 for _ in range(3)]
    for i in range(3):
        for k in range(3):
            s = 0.0
            for j in range(3):
                nj = n[j]
                if nj == 0.0:
                    continue
                for l in range(3):
                    s += t[i][j][k][l] * nj * n[l]
            g[i][k] = s
    return g


def jacobi_eigh(a, tol=1e-14, max_sweeps=100):
    """Eigenpairs of a real symmetric 3x3 matrix.

    Returns (eigenvalues, eigenvectors) sorted ascending by eigenvalue;
    eigenvectors[k] is the unit eigenvector for eigenvalues[k].
    """
    n = 3
    m = [row[:] for row in a]
    v = [[1.0 if i == j else 0.0 for j in range(n)] for i in range(n)]
    for _ in range(max_sweeps):
        off = max(abs(m[p][q]) for p in range(n) for q in range(p + 1, n))
        diag_max = max(abs(m[i][i]) for i in range(n)) or 1.0
        if off <= tol * diag_max:
            break
        for p in range(n):
            for q in range(p + 1, n):
                if abs(m[p][q]) <= 0.5 * tol * diag_max:
                    continue
                theta = 0.5 * (m[q][q] - m[p][p]) / m[p][q]
                t = math.copysign(1.0, theta) / (abs(theta) + math.sqrt(theta * theta + 1.0)) if theta != 0.0 else 1.0
                c = 1.0 / math.sqrt(t * t + 1.0)
                s = t * c
                app = m[p][p]
                aqq = m[q][q]
                apq = m[p][q]
                m[p][p] = app - t * apq
                m[q][q] = aqq + t * apq
                m[p][q] = m[q][p] = 0.0
                for r in range(n):
                    if r in (p, q):
                        continue
                    arp = m[r][p]
                    arq = m[r][q]
                    m[r][p] = m[p][r] = c * arp - s * arq
                    m[r][q] = m[q][r] = s * arp + c * arq
                for r in range(n):
                    vrp = v[r][p]
                    vrq = v[r][q]
                    v[r][p] = c * vrp - s * vrq
                    v[r][q] = s * vrp + c * vrq
    pairs = sorted(
        ((m[i][i], [v[0][i], v[1][i], v[2][i]]) for i in range(n)),
        key=lambda p: p[0],
    )
    return [p[0] for p in pairs], [p[1] for p in pairs]


def fix_sign(vec):
    """Make the largest-|component| positive; ties broken by axis order."""
    best = max(range(3), key=lambda i: (abs(vec[i]), -i))
    if vec[best] < 0.0:
        return [-x for x in vec]
    return list(vec)


def outer(u, v):
    return [[u[i] * v[j] for j in range(3)] for i in range(3)]


def mat_add(a, b):
    return [[a[i][j] + b[i][j] for j in range(3)] for i in range(3)]


@dataclass
class Material:
    density: float
    stiffness_voigt: list  # 6x6, crystal frame
    orientation: list  # 3x3, columns = crystal axes in lab frame

    @classmethod
    def from_config(cls, cfg):
        if not isinstance(cfg, dict):
            raise InputError("configuration root must be an object")
        try:
            density = cfg["density"]
            stiffness = cfg["stiffness"]
        except KeyError as exc:
            raise InputError(f"missing required key: {exc.args[0]}") from None
        return cls(
            density=validate_density(density),
            stiffness_voigt=validate_stiffness(stiffness),
            orientation=validate_orientation(cfg.get("orientation")),
        )

    def lab_tensor(self):
        crystal = voigt_to_tensor(self.stiffness_voigt)
        return rotate_tensor(crystal, self.orientation)


@dataclass
class ModeResult:
    index: int  # 1..3, sorted by ascending velocity
    eigenvalue: float
    velocity: float
    polarization: list | None  # unit vector in lab frame, None if degenerate
    degeneracy_group: int  # 0-based group id
    degeneracy_dimension: int
    residual: float  # ||Gamma v - lambda v||_inf


@dataclass
class DirectionResult:
    index: int
    direction: list  # normalized
    modes: list = field(default_factory=list)
    orthogonality_error: float = 0.0
    projections: dict = field(default_factory=dict)  # group id -> 3x3 matrix


def analyze(material, directions, degeneracy_rtol=1e-6):
    """Run the full modal analysis; returns a list of DirectionResult."""
    if not _is_number(degeneracy_rtol) or degeneracy_rtol < 0.0:
        raise InputError("degeneracy_rtol must be a non-negative finite number")
    if not directions:
        raise InputError("at least one propagation direction is required")
    tensor = material.lab_tensor()
    results = []
    for idx, raw in enumerate(directions):
        n = validate_direction(raw)
        results.append(_analyze_direction(tensor, material.density, idx, n, degeneracy_rtol))
    return results


def _analyze_direction(tensor, density, idx, n, rtol):
    gamma = christoffel(tensor, n)
    eigenvalues, eigenvectors = jacobi_eigh(gamma)
    lam_max = max(abs(x) for x in eigenvalues)
    if eigenvalues[0] < -1e-9 * max(lam_max, 1.0):
        raise InputError(
            "Christoffel matrix has a negative eigenvalue; the stiffness "
            "tensor does not support real wave speeds in this direction"
        )
    eigenvalues = [max(x, 0.0) for x in eigenvalues]

    groups = []
    for k, lam in enumerate(eigenvalues):
        if groups and lam - groups[-1][-1][0] <= rtol * max(lam_max, 1e-300):
            groups[-1].append((lam, k))
        else:
            groups.append([(lam, k)])

    result = DirectionResult(index=idx, direction=n)
    for gid, members in enumerate(groups):
        dim = len(members)
        if dim > 1:
            proj = [[0.0] * 3 for _ in range(3)]
            for _, k in members:
                proj = mat_add(proj, outer(eigenvectors[k], eigenvectors[k]))
            result.projections[gid] = proj
        for _, k in members:
            vec = eigenvectors[k]
            residual = max(
                abs(sum(gamma[i][j] * vec[j] for j in range(3)) - eigenvalues[k] * vec[i])
                for i in range(3)
            )
            result.modes.append(
                ModeResult(
                    index=len(result.modes) + 1,
                    eigenvalue=eigenvalues[k],
                    velocity=math.sqrt(eigenvalues[k] / density),
                    polarization=fix_sign(vec) if dim == 1 else None,
                    degeneracy_group=gid,
                    degeneracy_dimension=dim,
                    residual=residual,
                )
            )

    result.orthogonality_error = max(
        (
            abs(sum(eigenvectors[a][i] * eigenvectors[b][i] for i in range(3)))
            for a in range(3)
            for b in range(a + 1, 3)
        ),
        default=0.0,
    )
    return result
