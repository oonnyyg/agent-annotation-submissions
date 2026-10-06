"""Solve one load case: midplane strain/curvature and ply stresses."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .clt import assemble_abd, thermal_resultants, transform_alpha, transform_q
from .model import Laminate, LoadCase, Material


class SingularSystemError(RuntimeError):
    """Raised when the 6x6 ABD system cannot be solved."""


def solve_linear(matrix, rhs):
    """Gaussian elimination with partial pivoting (no dependencies)."""
    n = len(rhs)
    aug = [row[:] + [rhs[i]] for i, row in enumerate(matrix)]
    for col in range(n):
        pivot = max(range(col, n), key=lambda r: abs(aug[r][col]))
        if abs(aug[pivot][col]) < 1e-12 * max(
            1.0, max(abs(aug[r][c]) for r in range(n) for c in range(n))
        ):
            raise SingularSystemError(
                "ABD system is singular; check layup and material data"
            )
        aug[col], aug[pivot] = aug[pivot], aug[col]
        for r in range(col + 1, n):
            factor = aug[r][col] / aug[col][col]
            for c in range(col, n + 1):
                aug[r][c] -= factor * aug[col][c]
    x = [0.0] * n
    for r in range(n - 1, -1, -1):
        s = aug[r][n] - sum(aug[r][c] * x[c] for c in range(r + 1, n))
        x[r] = s / aug[r][r]
    return x


def rotate_stress_to_material(sigma_x, sigma_y, tau_xy, angle_deg):
    """Global (x,y) stress -> material (1,2) stress."""
    theta = math.radians(angle_deg)
    c = math.cos(theta)
    s = math.sin(theta)
    s1 = sigma_x * c * c + sigma_y * s * s + 2.0 * tau_xy * s * c
    s2 = sigma_x * s * s + sigma_y * c * c - 2.0 * tau_xy * s * c
    t12 = (sigma_y - sigma_x) * s * c + tau_xy * (c * c - s * s)
    return s1, s2, t12


@dataclass
class PlySurfaceStress:
    ply_index: int
    surface: str  # "bottom" or "top"
    z: float
    sigma_x: float
    sigma_y: float
    tau_xy: float
    sigma_1: float
    sigma_2: float
    tau_12: float


@dataclass
class CaseResult:
    name: str
    midplane_strain: list
    curvature: list
    a: list
    b: list
    d: list
    thermal_forces: list
    thermal_moments: list
    surface_stresses: list = field(default_factory=list)
    residual_forces: list = field(default_factory=list)
    residual_moments: list = field(default_factory=list)


def solve_case(laminate, materials, case):
    """Solve a single load case independently of any other case."""
    a, b, d = assemble_abd(laminate, materials)
    nt, mt = thermal_resultants(laminate, materials, case.delta_t)

    abd = [a[i] + b[i] for i in range(3)] + [b[i] + d[i] for i in range(3)]
    rhs = [
        case.nx + nt[0],
        case.ny + nt[1],
        case.nxy + nt[2],
        case.mx + mt[0],
        case.my + mt[1],
        case.mxy + mt[2],
    ]
    sol = solve_linear(abd, rhs)
    eps0, kappa = sol[:3], sol[3:]

    z = laminate.interfaces()
    result = CaseResult(
        name=case.name,
        midplane_strain=eps0,
        curvature=kappa,
        a=a, b=b, d=d,
        thermal_forces=nt,
        thermal_moments=mt,
    )

    int_n = [0.0, 0.0, 0.0]
    int_m = [0.0, 0.0, 0.0]
    for k, ply in enumerate(laminate.plies):
        mat = materials[ply.material]
        qb = transform_q(mat.q_matrix(), ply.angle_deg)
        alpha = transform_alpha(mat.alpha1, mat.alpha2, ply.angle_deg)
        for surface, zk in (("bottom", z[k]), ("top", z[k + 1])):
            mech = [
                eps0[j] + zk * kappa[j] - alpha[j] * case.delta_t
                for j in range(3)
            ]
            sig = [sum(qb[i][j] * mech[j] for j in range(3)) for i in range(3)]
            s1, s2, t12 = rotate_stress_to_material(
                sig[0], sig[1], sig[2], ply.angle_deg
            )
            result.surface_stresses.append(
                PlySurfaceStress(k, surface, zk, sig[0], sig[1], sig[2], s1, s2, t12)
            )
        # Reintegrate this ply's stress to cross-check overall equilibrium.
        z0, z1 = z[k], z[k + 1]
        for i in range(3):
            qb_a = sum(qb[i][j] * (eps0[j] - alpha[j] * case.delta_t) for j in range(3))
            qb_k = sum(qb[i][j] * kappa[j] for j in range(3))
            int_n[i] += qb_a * (z1 - z0) + qb_k * 0.5 * (z1 * z1 - z0 * z0)
            int_m[i] += (
                qb_a * 0.5 * (z1 * z1 - z0 * z0)
                + qb_k * (z1 ** 3 - z0 ** 3) / 3.0
            )

    applied_n = [case.nx, case.ny, case.nxy]
    applied_m = [case.mx, case.my, case.mxy]
    result.residual_forces = [int_n[i] - applied_n[i] for i in range(3)]
    result.residual_moments = [int_m[i] - applied_m[i] for i in range(3)]
    return result
