"""Classical laminate theory assembly, solve, and stress recovery."""

from __future__ import annotations

import math

from .material import Laminate


class SingularSystemError(RuntimeError):
    """Raised when the ABD system is singular."""


def _mat_vec(m: list[list[float]], v: list[float]) -> list[float]:
    return [sum(mi * vi for mi, vi in zip(row, v)) for row in m]


def _mat_mul(a: list[list[float]], b: list[list[float]]) -> list[list[float]]:
    return [
        [sum(a[i][k] * b[k][j] for k in range(len(b))) for j in range(len(b[0]))]
        for i in range(len(a))
    ]


def _transpose(m: list[list[float]]) -> list[list[float]]:
    return [list(row) for row in zip(*m)]


def transform_q(q: list[list[float]], angle_deg: float) -> list[list[float]]:
    """Rotate reduced stiffness from material axes to global axes."""
    t = math.radians(angle_deg)
    c, s = math.cos(t), math.sin(t)
    c2, s2, sc = c * c, s * s, s * c
    # Engineering-shear strain transformation: eps_material = t_eps eps_global.
    # Stiffness transforms as Qbar = t_eps^T Q t_eps.
    t_eps = [
        [c2, s2, sc],
        [s2, c2, -sc],
        [-2.0 * sc, 2.0 * sc, c2 - s2],
    ]
    return _mat_mul(_mat_mul(_transpose(t_eps), q), t_eps)


def transform_alpha(alpha: list[float], angle_deg: float) -> list[float]:
    """Rotate thermal expansion vector from material axes to global axes."""
    t = math.radians(angle_deg)
    c, s = math.cos(t), math.sin(t)
    a1, a2 = alpha[0], alpha[1]
    return [
        a1 * c * c + a2 * s * s,
        a1 * s * s + a2 * c * c,
        2.0 * (a1 - a2) * s * c,
    ]


def transform_stress_to_material(
    sigma_global: list[float], angle_deg: float
) -> list[float]:
    """Rotate a stress vector from global axes to material axes."""
    t = math.radians(angle_deg)
    c, s = math.cos(t), math.sin(t)
    sx, sy, txy = sigma_global
    return [
        sx * c * c + sy * s * s + 2.0 * txy * s * c,
        sx * s * s + sy * c * c - 2.0 * txy * s * c,
        (sy - sx) * s * c + txy * (c * c - s * s),
    ]


def solve_linear(a: list[list[float]], b: list[float]) -> list[float]:
    """Solve a dense linear system by Gaussian elimination with pivoting."""
    n = len(a)
    m = [row[:] + [b[i]] for i, row in enumerate(a)]
    for col in range(n):
        piv = max(range(col, n), key=lambda r: abs(m[r][col]))
        if abs(m[piv][col]) < 1e-12 * max(
            1.0, max(abs(m[r][c]) for r in range(n) for c in range(n))
        ):
            raise SingularSystemError("ABD system is singular")
        m[col], m[piv] = m[piv], m[col]
        for r in range(col + 1, n):
            f = m[r][col] / m[col][col]
            for c in range(col, n + 1):
                m[r][c] -= f * m[col][c]
    x = [0.0] * n
    for r in range(n - 1, -1, -1):
        x[r] = (m[r][n] - sum(m[r][c] * x[c] for c in range(r + 1, n))) / m[r][r]
    return x


def assemble_abd(
    laminate: Laminate,
) -> tuple[list[list[float]], list[float], list[float]]:
    """Assemble the 6x6 ABD matrix and thermal resultant vectors.

    Returns (abd, n_thermal, m_thermal) where n_thermal/m_thermal are the
    thermal force/moment resultants per unit temperature rise.
    """
    laminate.validate()
    zs = laminate.interfaces()
    abd = [[0.0] * 6 for _ in range(6)]
    n_th = [0.0] * 3
    m_th = [0.0] * 3
    for k, layer in enumerate(laminate.layers):
        z0, z1 = zs[k], zs[k + 1]
        dz = z1 - z0
        dz2 = (z1**2 - z0**2) / 2.0
        dz3 = (z1**3 - z0**3) / 3.0
        qb = transform_q(layer.material.q_matrix(), layer.angle_deg)
        ag = transform_alpha(layer.material.alpha_vector(), layer.angle_deg)
        q_alpha = _mat_vec(qb, ag)
        for i in range(3):
            for j in range(3):
                abd[i][j] += qb[i][j] * dz
                abd[i][j + 3] += qb[i][j] * dz2
                abd[i + 3][j] += qb[i][j] * dz2
                abd[i + 3][j + 3] += qb[i][j] * dz3
            n_th[i] += q_alpha[i] * dz
            m_th[i] += q_alpha[i] * dz2
    return abd, n_th, m_th


def solve_load_case(
    laminate: Laminate,
    forces: list[float],
    moments: list[float],
    delta_t: float,
) -> tuple[list[float], list[float]]:
    """Solve one load case; returns (midplane strain, curvature)."""
    abd, n_th, m_th = assemble_abd(laminate)
    rhs = [forces[i] + delta_t * n_th[i] for i in range(3)] + [
        moments[i] + delta_t * m_th[i] for i in range(3)
    ]
    sol = solve_linear(abd, rhs)
    return sol[:3], sol[3:]


def layer_surface_stresses(
    laminate: Laminate,
    eps0: list[float],
    kappa: list[float],
    delta_t: float,
) -> list[dict]:
    """Recover global and material-axis stresses at each layer surface."""
    zs = laminate.interfaces()
    results = []
    for k, layer in enumerate(laminate.layers):
        qb = transform_q(layer.material.q_matrix(), layer.angle_deg)
        ag = transform_alpha(layer.material.alpha_vector(), layer.angle_deg)
        surfaces = []
        for label, z in (("bottom", zs[k]), ("top", zs[k + 1])):
            mech = [
                eps0[i] + z * kappa[i] - delta_t * ag[i] for i in range(3)
            ]
            sig_g = _mat_vec(qb, mech)
            sig_m = transform_stress_to_material(sig_g, layer.angle_deg)
            surfaces.append(
                {
                    "surface": label,
                    "z": z,
                    "stress_global": sig_g,
                    "stress_material": sig_m,
                }
            )
        results.append({"layer_index": k + 1, "surfaces": surfaces})
    return results


def integrate_resultants(
    laminate: Laminate,
    eps0: list[float],
    kappa: list[float],
    delta_t: float,
) -> tuple[list[float], list[float]]:
    """Integrate layer stresses to recover force and moment resultants."""
    zs = laminate.interfaces()
    n = [0.0] * 3
    m = [0.0] * 3
    for k, layer in enumerate(laminate.layers):
        qb = transform_q(layer.material.q_matrix(), layer.angle_deg)
        ag = transform_alpha(layer.material.alpha_vector(), layer.angle_deg)
        z0, z1 = zs[k], zs[k + 1]
        dz = z1 - z0
        dz2 = (z1**2 - z0**2) / 2.0
        for i in range(3):
            for j in range(3):
                coef = eps0[j] - delta_t * ag[j]
                n[i] += qb[i][j] * (coef * dz + kappa[j] * dz2)
                m[i] += qb[i][j] * (coef * dz2 + kappa[j] * (z1**3 - z0**3) / 3.0)
    return n, m


def analyze_laminate(
    laminate: Laminate,
    load_cases: list[dict],
) -> list[dict]:
    """Run all load cases independently and return per-case results."""
    abd, n_th, m_th = assemble_abd(laminate)
    cases = []
    for case in load_cases:
        forces = [float(v) for v in case.get("forces", [0.0, 0.0, 0.0])]
        moments = [float(v) for v in case.get("moments", [0.0, 0.0, 0.0])]
        delta_t = float(case.get("delta_t", 0.0))
        eps0, kappa = solve_load_case(laminate, forces, moments, delta_t)
        layers = layer_surface_stresses(laminate, eps0, kappa, delta_t)
        n_int, m_int = integrate_resultants(laminate, eps0, kappa, delta_t)
        resid_n = [n_int[i] - forces[i] for i in range(3)]
        resid_m = [m_int[i] - moments[i] for i in range(3)]
        cases.append(
            {
                "name": case.get("name", f"case_{len(cases) + 1}"),
                "midplane_strain": eps0,
                "curvature": kappa,
                "layers": layers,
                "equilibrium_residual": {"forces": resid_n, "moments": resid_m},
            }
        )
    return cases
