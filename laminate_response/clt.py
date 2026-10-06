"""Classical laminate theory assembly: A, B, D and thermal resultants."""

from __future__ import annotations

import math

from .model import Laminate, Material


def transform_q(q, angle_deg):
    """Rotate reduced stiffness from material axes to global (x, y).

    Angle is measured counterclockwise from global x to material 1 axis.
    Engineering shear strain is used throughout.
    """
    theta = math.radians(angle_deg)
    c = math.cos(theta)
    s = math.sin(theta)
    c2, s2 = c * c, s * s
    q11, q12, q22, q66 = q[0][0], q[0][1], q[1][1], q[2][2]
    qb = [[0.0] * 3 for _ in range(3)]
    qb[0][0] = q11 * c2 * c2 + 2.0 * (q12 + 2.0 * q66) * s2 * c2 + q22 * s2 * s2
    qb[0][1] = (q11 + q22 - 4.0 * q66) * s2 * c2 + q12 * (c2 * c2 + s2 * s2)
    qb[1][1] = q11 * s2 * s2 + 2.0 * (q12 + 2.0 * q66) * s2 * c2 + q22 * c2 * c2
    qb[2][2] = (q11 + q22 - 2.0 * q12 - 2.0 * q66) * s2 * c2 + q66 * (c2 * c2 + s2 * s2)
    qb[0][2] = (q11 - q12 - 2.0 * q66) * c2 * c * s - (q22 - q12 - 2.0 * q66) * s2 * s * c
    qb[1][2] = (q11 - q12 - 2.0 * q66) * c * s2 * s - (q22 - q12 - 2.0 * q66) * c2 * c * s
    qb[1][0] = qb[0][1]
    qb[2][0] = qb[0][2]
    qb[2][1] = qb[1][2]
    return qb


def transform_alpha(alpha1, alpha2, angle_deg):
    """Thermal expansion vector in global axes (engineering shear)."""
    theta = math.radians(angle_deg)
    c = math.cos(theta)
    s = math.sin(theta)
    return [
        alpha1 * c * c + alpha2 * s * s,
        alpha1 * s * s + alpha2 * c * c,
        2.0 * (alpha1 - alpha2) * s * c,
    ]


def assemble_abd(laminate, materials):
    """Assemble A, B, D matrices about the midplane (z = 0)."""
    a = [[0.0] * 3 for _ in range(3)]
    b = [[0.0] * 3 for _ in range(3)]
    d = [[0.0] * 3 for _ in range(3)]
    z = laminate.interfaces()
    for k, ply in enumerate(laminate.plies):
        qb = transform_q(materials[ply.material].q_matrix(), ply.angle_deg)
        z0, z1 = z[k], z[k + 1]
        da = z1 - z0
        db = 0.5 * (z1 * z1 - z0 * z0)
        dd = (z1 ** 3 - z0 ** 3) / 3.0
        for i in range(3):
            for j in range(3):
                a[i][j] += qb[i][j] * da
                b[i][j] += qb[i][j] * db
                d[i][j] += qb[i][j] * dd
    return a, b, d


def thermal_resultants(laminate, materials, delta_t):
    """Thermal force and moment resultants N^T, M^T for a uniform dT."""
    nt = [0.0, 0.0, 0.0]
    mt = [0.0, 0.0, 0.0]
    z = laminate.interfaces()
    for k, ply in enumerate(laminate.plies):
        mat = materials[ply.material]
        qb = transform_q(mat.q_matrix(), ply.angle_deg)
        alpha = transform_alpha(mat.alpha1, mat.alpha2, ply.angle_deg)
        z0, z1 = z[k], z[k + 1]
        da = z1 - z0
        db = 0.5 * (z1 * z1 - z0 * z0)
        for i in range(3):
            qa = sum(qb[i][j] * alpha[j] for j in range(3))
            nt[i] += qa * da * delta_t
            mt[i] += qa * db * delta_t
    return nt, mt
