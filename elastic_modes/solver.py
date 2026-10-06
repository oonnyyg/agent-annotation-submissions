"""Christoffel equation solver for elastic wave modes."""

import math

from .errors import InputError
from .tensor import VOIGT_PAIRS


def voigt_to_tensor(c):
    """Expand a 6x6 Voigt stiffness matrix to the full 3x3x3x3 tensor."""
    t = [[[[0.0] * 3 for _ in range(3)] for _ in range(3)] for _ in range(3)]
    for a in range(6):
        i, j = VOIGT_PAIRS[a]
        for b in range(6):
            k, l = VOIGT_PAIRS[b]
            t[i][j][k][l] = c[a][b]
            t[j][i][k][l] = c[a][b]
            t[i][j][l][k] = c[a][b]
            t[j][i][l][k] = c[a][b]
    return t


def christoffel(t, n):
    """Gamma_ik = c_ijkl n_j n_l for unit direction n."""
    g = [[0.0] * 3 for _ in range(3)]
    for i in range(3):
        for k in range(3):
            s = 0.0
            for j in range(3):
                for l in range(3):
                    s += t[i][j][k][l] * n[j] * n[l]
            g[i][k] = s
    return g


def jacobi_eigen(a):
    """Eigenpairs of a symmetric 3x3 matrix via cyclic Jacobi sweeps."""
    m = [row[:] for row in a]
    v = [[1.0 if i == j else 0.0 for j in range(3)] for i in range(3)]
    for _ in range(64):
        off = max(abs(m[0][1]), abs(m[0][2]), abs(m[1][2]))
        if off <= 1e-15 * max(1.0, abs(m[0][0]), abs(m[1][1]), abs(m[2][2])):
            break
        for p, q in ((0, 1), (0, 2), (1, 2)):
            if m[p][q] == 0.0:
                continue
            theta = 0.5 * math.atan2(2.0 * m[p][q], m[q][q] - m[p][p])
            c, s = math.cos(theta), math.sin(theta)
            for k in range(3):
                mkp, mkq = m[k][p], m[k][q]
                m[k][p] = c * mkp - s * mkq
                m[k][q] = s * mkp + c * mkq
            for k in range(3):
                mpk, mqk = m[p][k], m[q][k]
                m[p][k] = c * mpk - s * mqk
                m[q][k] = s * mpk + c * mqk
            for k in range(3):
                vkp, vkq = v[k][p], v[k][q]
                v[k][p] = c * vkp - s * vkq
                v[k][q] = s * vkp + c * vkq
    values = [m[i][i] for i in range(3)]
    vectors = [[v[i][j] for i in range(3)] for j in range(3)]
    return values, vectors


def _fix_sign(vec):
    """Make the largest-|component| positive; ties resolved by coordinate order."""
    best = max(range(3), key=lambda i: (abs(vec[i]), -i))
    if vec[best] < 0.0:
        return [-x for x in vec]
    return list(vec)


def _rotate_voigt(c, q):
    """Rotate a Voigt stiffness into the lab frame via the full tensor."""
    t = voigt_to_tensor(c)
    r = [[[[0.0] * 3 for _ in range(3)] for _ in range(3)] for _ in range(3)]
    for i in range(3):
        for j in range(3):
            for k in range(3):
                for l in range(3):
                    s = 0.0
                    for a in range(3):
                        for b in range(3):
                            for cc in range(3):
                                for d in range(3):
                                    s += (
                                        q[i][a] * q[j][b] * q[k][cc] * q[l][d]
                                        * t[a][b][cc][d]
                                    )
                    r[i][j][k][l] = s
    out = [[0.0] * 6 for _ in range(6)]
    for a in range(6):
        i, j = VOIGT_PAIRS[a]
        for b in range(6):
            k, l = VOIGT_PAIRS[b]
            out[a][b] = r[i][j][k][l]
    return out


def solve_modes(c_voigt, density, direction, orientation, rel_tol=1e-6):
    """Solve the Christoffel equation for one lab-frame direction.

    The direction is rotated into the crystal frame (n_xtal = Q^T n_lab),
    and eigenvectors are rotated back (p_lab = Q p_xtal), so wave speeds and
    polarization subspaces are invariant under the choice of coordinates.
    Returns a dict with modes sorted by ascending wave speed plus diagnostics.
    """
    n_lab = direction
    n_xtal = [sum(orientation[k][i] * n_lab[k] for k in range(3)) for i in range(3)]
    t = voigt_to_tensor(c_voigt)
    gamma = christoffel(t, n_xtal)
    values, vectors = jacobi_eigen(gamma)

    order = sorted(range(3), key=lambda i: values[i])
    values = [values[i] for i in order]
    vectors = [vectors[i] for i in order]

    lam_max = max(values)
    for lam in values:
        if lam <= 0.0:
            raise InputError(
                f"non-positive Christoffel eigenvalue {lam}; "
                "refusing to report a wave speed"
            )

    lab_vectors = [
        [sum(orientation[i][k] * vec[k] for k in range(3)) for i in range(3)]
        for vec in vectors
    ]

    groups = []
    for idx, lam in enumerate(values):
        if groups and lam - groups[-1]["values"][-1] <= rel_tol * lam_max:
            groups[-1]["indices"].append(idx)
            groups[-1]["values"].append(lam)
        else:
            groups.append({"indices": [idx], "values": [lam]})

    gamma_lab = christoffel(voigt_to_tensor(_rotate_voigt(c_voigt, orientation)), n_lab)
    residuals = []
    for lam, p in zip(values, lab_vectors):
        r = [sum(gamma_lab[i][k] * p[k] for k in range(3)) - lam * p[i] for i in range(3)]
        residuals.append(math.sqrt(sum(x * x for x in r)) / lam_max)
    ortho_err = 0.0
    for a in range(3):
        for b in range(3):
            d = sum(lab_vectors[a][i] * lab_vectors[b][i] for i in range(3))
            ortho_err = max(ortho_err, abs(d - (1.0 if a == b else 0.0)))

    modes = []
    for group in groups:
        idxs = group["indices"]
        mean_lam = sum(group["values"]) / len(idxs)
        entry = {
            "mode_indices": idxs,
            "eigenvalue": mean_lam,
            "velocity_m_per_s": math.sqrt(mean_lam / density),
            "degeneracy": len(idxs),
        }
        if len(idxs) == 1:
            i = idxs[0]
            entry["polarization"] = _fix_sign(lab_vectors[i])
            entry["eigen_residual"] = residuals[i]
        else:
            proj = [[0.0] * 3 for _ in range(3)]
            for i in idxs:
                p = lab_vectors[i]
                for a in range(3):
                    for b in range(3):
                        proj[a][b] += p[a] * p[b]
            entry["projector"] = proj
            entry["eigen_residual"] = max(residuals[i] for i in idxs)
        modes.append(entry)

    return {
        "direction": n_lab,
        "modes": modes,
        "orthogonality_error": ortho_err,
    }
