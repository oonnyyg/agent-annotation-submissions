import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from elastic_modes.errors import InputError
from elastic_modes.solver import solve_modes
from elastic_modes.tensor import (
    normalize_direction,
    validate_orientation,
    validate_stiffness,
)

RHO = 2700.0
C11, C12, C44 = 108.2e9, 61.3e9, 23.45e9


def isotropic_c():
    return [
        [C11, C12, C12, 0, 0, 0],
        [C12, C11, C12, 0, 0, 0],
        [C12, C12, C11, 0, 0, 0],
        [0, 0, 0, C44, 0, 0],
        [0, 0, 0, 0, C44, 0],
        [0, 0, 0, 0, 0, C44],
    ]


def cubic_c():
    # Copper-like cubic crystal (distinct C11, C12, C44 -> anisotropic).
    c11, c12, c44 = 168.4e9, 121.4e9, 75.4e9
    return [
        [c11, c12, c12, 0, 0, 0],
        [c12, c11, c12, 0, 0, 0],
        [c12, c12, c11, 0, 0, 0],
        [0, 0, 0, c44, 0, 0],
        [0, 0, 0, 0, c44, 0],
        [0, 0, 0, 0, 0, c44],
    ]


IDENTITY = [[1.0, 0.0, 0.0], [0.0, 1.0, 0.0], [0.0, 0.0, 1.0]]


def rot_z(deg):
    a = math.radians(deg)
    c, s = math.cos(a), math.sin(a)
    return [[c, -s, 0.0], [s, c, 0.0], [0.0, 0.0, 1.0]]


class TestIsotropic(unittest.TestCase):
    def test_double_shear_degeneracy(self):
        for d in ([1, 0, 0], [1, 2, 3], [0, 0, -5]):
            res = solve_modes(isotropic_c(), RHO, normalize_direction(d), IDENTITY)
            self.assertEqual(len(res["modes"]), 2)
            shear, longi = res["modes"]
            self.assertEqual(shear["degeneracy"], 2)
            self.assertIn("projector", shear)
            self.assertNotIn("polarization", shear)
            self.assertAlmostEqual(
                shear["velocity_m_per_s"], math.sqrt(C44 / RHO), places=6
            )
            self.assertAlmostEqual(
                longi["velocity_m_per_s"], math.sqrt(C11 / RHO), places=6
            )
            self.assertLess(res["orthogonality_error"], 1e-12)

    def test_longitudinal_polarization_parallel(self):
        d = normalize_direction([1, 2, 3])
        res = solve_modes(isotropic_c(), RHO, d, IDENTITY)
        pol = res["modes"][1]["polarization"]
        for p, n in zip(pol, d):
            self.assertAlmostEqual(p, n, places=12)

    def test_projector_is_identity_minus_nn(self):
        d = normalize_direction([1, 0, 1])
        res = solve_modes(isotropic_c(), RHO, d, IDENTITY)
        proj = res["modes"][0]["projector"]
        for i in range(3):
            for j in range(3):
                expect = (1.0 if i == j else 0.0) - d[i] * d[j]
                self.assertAlmostEqual(proj[i][j], expect, places=10)


class TestRotationInvariance(unittest.TestCase):
    def test_isotropic_speeds_rotation_invariant(self):
        # For an isotropic material, any crystal orientation must give
        # identical speeds: coordinate choice cannot create anisotropy.
        q = rot_z(37.0)
        d_lab = normalize_direction([0.3, 0.5, 0.8])
        base = solve_modes(isotropic_c(), RHO, d_lab, IDENTITY)
        rot = solve_modes(isotropic_c(), RHO, d_lab, q)
        for m1, m2 in zip(base["modes"], rot["modes"]):
            self.assertAlmostEqual(
                m1["velocity_m_per_s"], m2["velocity_m_per_s"], places=6
            )
            self.assertEqual(m1["degeneracy"], m2["degeneracy"])

    def test_rotated_direction_matches_rotated_crystal(self):
        # Rotating crystal by Q and direction by Q leaves crystal-frame
        # physics unchanged; speeds must match exactly.
        q = rot_z(63.0)
        c = cubic_c()
        d = normalize_direction([1.0, 0.0, 0.0])
        d_rot = [sum(q[i][k] * d[k] for k in range(3)) for i in range(3)]
        base = solve_modes(c, RHO, d, IDENTITY)
        rot = solve_modes(c, RHO, d_rot, q)
        for m1, m2 in zip(base["modes"], rot["modes"]):
            self.assertAlmostEqual(
                m1["velocity_m_per_s"], m2["velocity_m_per_s"], places=3
            )


class TestValidation(unittest.TestCase):
    def test_asymmetric_stiffness_rejected(self):
        c = isotropic_c()
        c[0][1] += 1e9
        with self.assertRaises(InputError):
            validate_stiffness(c)

    def test_non_positive_definite_rejected(self):
        c = isotropic_c()
        c[3][3] = -1.0
        with self.assertRaises(InputError):
            validate_stiffness(c)

    def test_bad_orientation_rejected(self):
        with self.assertRaises(InputError):
            validate_orientation([[1, 0, 0], [0, 1, 0], [0, 0, -1]])  # det = -1
        with self.assertRaises(InputError):
            validate_orientation([[2, 0, 0], [0, 1, 0], [0, 0, 1]])  # not orthogonal

    def test_zero_direction_rejected(self):
        with self.assertRaises(InputError):
            normalize_direction([0, 0, 0])

    def test_cli_rejects_bad_density(self):
        cfg = {"density": -1, "stiffness": isotropic_c(), "directions": [[1, 0, 0]]}
        with tempfile.TemporaryDirectory() as tmp:
            path = os.path.join(tmp, "cfg.json")
            with open(path, "w", encoding="utf-8") as fh:
                json.dump(cfg, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "elastic_modes", path,
                 "--csv", os.path.join(tmp, "o.csv"),
                 "--json", os.path.join(tmp, "o.json")],
                capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 2)
            self.assertFalse(os.path.exists(os.path.join(tmp, "o.csv")))


if __name__ == "__main__":
    unittest.main()
