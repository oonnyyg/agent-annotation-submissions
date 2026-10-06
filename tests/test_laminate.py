import math
import unittest

from laminate_response import Material, Layer, Laminate, analyze_laminate
from laminate_response.clt import (
    SingularSystemError,
    assemble_abd,
    solve_linear,
    solve_load_case,
    transform_q,
)
from laminate_response.material import InputError


def make_material():
    return Material(
        name="t300",
        e1=181e9,
        e2=10.3e9,
        nu12=0.28,
        g12=7.17e9,
        alpha1=0.02e-6,
        alpha2=22.5e-6,
    )


def make_laminate(angles, thickness=0.125e-3):
    mat = make_material()
    return Laminate(tuple(Layer(mat, thickness, a) for a in angles))


class MaterialValidationTests(unittest.TestCase):
    def test_nonpositive_modulus_rejected(self):
        mat = Material("bad", -1.0, 10e9, 0.28, 7e9, 0.0, 0.0)
        with self.assertRaises(InputError):
            mat.validate()

    def test_non_positive_definite_rejected(self):
        mat = Material("bad", 10e9, 10e9, 1.5, 7e9, 0.0, 0.0)
        with self.assertRaises(InputError):
            mat.validate()

    def test_zero_thickness_rejected(self):
        with self.assertRaises(InputError):
            Layer(make_material(), 0.0, 0.0).validate()


class StiffnessTests(unittest.TestCase):
    def test_transform_zero_angle_is_identity(self):
        q = make_material().q_matrix()
        qb = transform_q(q, 0.0)
        for i in range(3):
            for j in range(3):
                self.assertAlmostEqual(qb[i][j], q[i][j], places=3)

    def test_symmetric_laminate_has_zero_b(self):
        lam = make_laminate([0, 45, -45, 90, 90, -45, 45, 0])
        abd, _, _ = assemble_abd(lam)
        for i in range(3):
            for j in range(3):
                self.assertAlmostEqual(abd[i][j + 3], 0.0, places=2)

    def test_reversed_layup_flips_b_sign(self):
        abd1, _, _ = assemble_abd(make_laminate([0, 90]))
        abd2, _, _ = assemble_abd(make_laminate([90, 0]))
        for i in range(3):
            for j in range(3):
                self.assertAlmostEqual(abd1[i][j], abd2[i][j], places=2)
                self.assertAlmostEqual(abd1[i][j + 3], -abd2[i][j + 3], places=2)
                self.assertAlmostEqual(
                    abd1[i + 3][j + 3], abd2[i + 3][j + 3], places=2
                )

    def test_singular_system_raises(self):
        with self.assertRaises(SingularSystemError):
            solve_linear([[0.0, 0.0], [0.0, 0.0]], [1.0, 2.0])


class ResponseTests(unittest.TestCase):
    def test_symmetric_tension_no_curvature(self):
        lam = make_laminate([0, 45, -45, 90, 90, -45, 45, 0])
        eps0, kappa = solve_load_case(lam, [1e5, 0.0, 0.0], [0.0] * 3, 0.0)
        self.assertGreater(eps0[0], 0.0)
        self.assertLess(eps0[1], 0.0)
        for k in kappa:
            self.assertAlmostEqual(k, 0.0, places=12)

    def test_free_thermal_symmetric_is_flat(self):
        lam = make_laminate([0, 90, 90, 0])
        eps0, kappa = solve_load_case(lam, [0.0] * 3, [0.0] * 3, -100.0)
        self.assertLess(eps0[0], 0.0)
        self.assertAlmostEqual(eps0[0], eps0[1], places=12)
        for k in kappa:
            self.assertAlmostEqual(k, 0.0, places=12)

    def test_free_thermal_unsymmetric_curves(self):
        lam = make_laminate([0, 90])
        eps0, kappa = solve_load_case(lam, [0.0] * 3, [0.0] * 3, -100.0)
        self.assertNotAlmostEqual(kappa[0], 0.0, places=6)
        self.assertAlmostEqual(kappa[0], -kappa[1], places=12)

    def test_reversed_layup_opposite_curvature(self):
        _, k1 = solve_load_case(make_laminate([0, 90]), [0.0] * 3, [0.0] * 3, -100.0)
        _, k2 = solve_load_case(make_laminate([90, 0]), [0.0] * 3, [0.0] * 3, -100.0)
        for a, b in zip(k1, k2):
            self.assertAlmostEqual(a, -b, places=12)

    def test_equilibrium_residual_is_small(self):
        lam = make_laminate([30, -60, 0, 90])
        (case,) = analyze_laminate(
            lam,
            [{"name": "combined", "forces": [1e4, -2e4, 5e3],
              "moments": [10.0, -5.0, 2.0], "delta_t": 75.0}],
        )
        for value in case["equilibrium_residual"]["forces"]:
            self.assertAlmostEqual(value, 0.0, places=4)
        for value in case["equilibrium_residual"]["moments"]:
            self.assertAlmostEqual(value, 0.0, places=8)

    def test_cases_are_independent(self):
        lam = make_laminate([0, 90])
        cases = analyze_laminate(
            lam, [{"name": "a", "delta_t": 50.0}, {"name": "b"}]
        )
        for value in cases[1]["midplane_strain"] + cases[1]["curvature"]:
            self.assertEqual(value, 0.0)

    def test_unidirectional_thermal_strain(self):
        lam = make_laminate([0], thickness=1.0)
        eps0, _ = solve_load_case(lam, [0.0] * 3, [0.0] * 3, 100.0)
        self.assertAlmostEqual(eps0[0], 0.02e-6 * 100.0, places=14)
        self.assertAlmostEqual(eps0[1], 22.5e-6 * 100.0, places=12)


if __name__ == "__main__":
    unittest.main()
