import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from elastic_modes.core import InputError, Material, analyze


def isotropic_stiffness(c11, c44):
    c12 = c11 - 2.0 * c44
    m = [[0.0] * 6 for _ in range(6)]
    for i in range(3):
        m[i][i] = c11
        m[i + 3][i + 3] = c44
    for i in range(3):
        for j in range(3):
            if i != j:
                m[i][j] = c12
    return m


def rotation_z(deg):
    a = math.radians(deg)
    c, s = math.cos(a), math.sin(a)
    return [[c, -s, 0.0], [s, c, 0.0], [0.0, 0.0, 1.0]]


def rotation_x(deg):
    a = math.radians(deg)
    c, s = math.cos(a), math.sin(a)
    return [[1.0, 0.0, 0.0], [0.0, c, -s], [0.0, s, c]]


def mat_vec(m, v):
    return [sum(m[i][j] * v[j] for j in range(3)) for i in range(3)]


class IsotropicTests(unittest.TestCase):
    def setUp(self):
        self.mat = Material(
            density=2700.0,
            stiffness_voigt=isotropic_stiffness(108.0e9, 26.0e9),
            orientation=[[1.0, 0.0, 0.0], [0.0, 1.0, 0.0], [0.0, 0.0, 1.0]],
        )

    def test_shear_doublet_degenerate(self):
        (res,) = analyze(self.mat, [[0.3, 0.5, 0.8062257748]], degeneracy_rtol=1e-9)
        speeds = [m.velocity for m in res.modes]
        self.assertTrue(speeds == sorted(speeds))
        vs = math.sqrt(26.0e9 / 2700.0)
        vl = math.sqrt(108.0e9 / 2700.0)
        self.assertAlmostEqual(speeds[0], vs, delta=vs * 1e-9)
        self.assertAlmostEqual(speeds[1], vs, delta=vs * 1e-9)
        self.assertAlmostEqual(speeds[2], vl, delta=vl * 1e-9)
        self.assertEqual(res.modes[0].degeneracy_dimension, 2)
        self.assertEqual(res.modes[1].degeneracy_dimension, 2)
        self.assertIsNone(res.modes[0].polarization)
        self.assertIsNone(res.modes[1].polarization)
        self.assertIsNotNone(res.modes[2].polarization)
        proj = res.projections[res.modes[0].degeneracy_group]
        n = res.direction
        # Projection onto shear subspace annihilates n (longitudinal dir).
        pn = mat_vec(proj, n)
        self.assertLess(max(abs(x) for x in pn), 1e-9)
        self.assertLess(res.orthogonality_error, 1e-12)
        self.assertLess(max(m.residual for m in res.modes), 1e-3)

    def test_longitudinal_polarization_parallel(self):
        (res,) = analyze(self.mat, [[0.0, 0.0, 2.5]])
        pol = res.modes[2].polarization
        self.assertAlmostEqual(abs(pol[2]), 1.0, places=12)
        self.assertGreater(pol[2], 0.0)


class RotationTests(unittest.TestCase):
    def setUp(self):
        # Cubic-like anisotropic stiffness (c11 != c12 + 2 c44).
        c = [[0.0] * 6 for _ in range(6)]
        for i in range(3):
            c[i][i] = 160.0e9
            c[i + 3][i + 3] = 80.0e9
        for i in range(3):
            for j in range(3):
                if i != j:
                    c[i][j] = 60.0e9
        self.c = c
        self.rho = 4400.0
        self.eye = [[1.0, 0.0, 0.0], [0.0, 1.0, 0.0], [0.0, 0.0, 1.0]]

    def speeds(self, orientation, direction):
        mat = Material(self.rho, self.c, orientation)
        (res,) = analyze(mat, [direction])
        return [m.velocity for m in res.modes], res

    def test_rotated_crystal_matches_rotated_direction(self):
        q = rotation_z(37.0)
        # Rotating the crystal by Q and probing n is equivalent to
        # probing Q^T n with the unrotated crystal.
        s_rot, _ = self.speeds(q, [1.0, 0.0, 0.0])
        qt_n = mat_vec([[q[j][i] for j in range(3)] for i in range(3)], [1.0, 0.0, 0.0])
        s_ref, _ = self.speeds(self.eye, qt_n)
        for a, b in zip(s_rot, s_ref):
            self.assertAlmostEqual(a, b, delta=max(a, b) * 1e-9)

    def test_rotation_invariance_of_projection(self):
        q = rotation_x(25.0)
        _, res_rot = self.speeds(q, [0.0, 1.0, 0.0])
        qt = [[q[j][i] for j in range(3)] for i in range(3)]
        _, res_ref = self.speeds(self.eye, mat_vec(qt, [0.0, 1.0, 0.0]))
        self.assertEqual(
            sorted(m.degeneracy_dimension for m in res_rot.modes),
            sorted(m.degeneracy_dimension for m in res_ref.modes),
        )

    def test_non_axis_direction(self):
        s, res = self.speeds(rotation_z(15.0), [1.0, 2.0, 3.0])
        self.assertEqual(len(s), 3)
        self.assertTrue(all(v > 0 for v in s))
        self.assertLess(res.orthogonality_error, 1e-12)


class ValidationTests(unittest.TestCase):
    def base(self):
        return dict(density=1000.0, stiffness=isotropic_stiffness(10e9, 4e9))

    def material(self, **kw):
        cfg = self.base()
        cfg.update(kw)
        return Material.from_config(cfg)

    def test_negative_density_rejected(self):
        with self.assertRaises(InputError):
            self.material(density=-1.0)

    def test_non_symmetric_stiffness_rejected(self):
        c = isotropic_stiffness(10e9, 4e9)
        c[0][1] = 5e9
        with self.assertRaises(InputError):
            self.material(stiffness=c)

    def test_non_spd_stiffness_rejected(self):
        c = isotropic_stiffness(10e9, 4e9)
        c[0][0] = -1.0
        with self.assertRaises(InputError):
            self.material(stiffness=c)

    def test_bad_orientation_rejected(self):
        with self.assertRaises(InputError):
            self.material(orientation=[[1, 0, 0], [0, 1, 0], [0, 0, -1]])
        with self.assertRaises(InputError):
            self.material(orientation=[[2, 0, 0], [0, 1, 0], [0, 0, 1]])

    def test_zero_direction_rejected(self):
        with self.assertRaises(InputError):
            analyze(self.material(), [[0.0, 0.0, 0.0]])

    def test_negative_eigenvalue_not_absorbed(self):
        # SPD Voigt implies strong ellipticity, so a negative Christoffel
        # eigenvalue cannot arise from validated input; verify the guard
        # directly at the analysis layer (no abs() trickery allowed).
        from elastic_modes.core import _analyze_direction

        tensor = [[[[0.0] * 3 for _ in range(3)] for _ in range(3)] for _ in range(3)]
        tensor[0][0][0][0] = -1.0e9
        with self.assertRaises(InputError):
            _analyze_direction(tensor, 1000.0, 0, [1.0, 0.0, 0.0], 1e-6)


class CliTests(unittest.TestCase):
    def test_cli_end_to_end(self):
        cfg = {
            "material": {
                "density": 2700.0,
                "stiffness": isotropic_stiffness(108.0e9, 26.0e9),
            },
            "orientation": rotation_z(30.0),
            "directions": [[1, 0, 0], [0, 0, 1], [1, 1, 1]],
            "degeneracy_rtol": 1e-6,
        }
        with tempfile.TemporaryDirectory() as tmp:
            cfg_path = os.path.join(tmp, "cfg.json")
            csv_path = os.path.join(tmp, "out.csv")
            diag_path = os.path.join(tmp, "diag.json")
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump(cfg, fh)
            proc = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "elastic_modes",
                    cfg_path,
                    "--velocities",
                    csv_path,
                    "--diagnostics",
                    diag_path,
                ],
                capture_output=True,
                text=True,
                cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(csv_path, encoding="utf-8") as fh:
                rows = fh.read().strip().splitlines()
            self.assertEqual(len(rows), 1 + 9)
            with open(diag_path, encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertEqual(len(diag["directions"]), 3)
            for entry in diag["directions"]:
                self.assertLess(entry["orthogonality_error"], 1e-12)

    def test_cli_rejects_invalid_config(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg_path = os.path.join(tmp, "bad.json")
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump({"density": -5, "stiffness": [], "directions": [[1, 0, 0]]}, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "elastic_modes", cfg_path],
                capture_output=True,
                text=True,
                cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            )
            self.assertEqual(proc.returncode, 2)
            self.assertIn("error:", proc.stderr)


if __name__ == "__main__":
    unittest.main()
