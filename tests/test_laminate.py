import math
import unittest

from laminate_response.model import Laminate, LoadCase, Material, Ply
from laminate_response.clt import assemble_abd, transform_q
from laminate_response.solver import solve_case


def make_material():
    return {
        "t300": Material(
            name="t300",
            e1=181e9,
            e2=10.3e9,
            nu12=0.28,
            g12=7.17e9,
            alpha1=0.02e-6,
            alpha2=22.5e-6,
        )
    }


def make_laminate(angles, t=0.125e-3):
    return Laminate(tuple(Ply("t300", a, t) for a in angles))


def zero_case(**kw):
    vals = dict(name="c", nx=0.0, ny=0.0, nxy=0.0,
                mx=0.0, my=0.0, mxy=0.0, delta_t=0.0)
    vals.update(kw)
    return LoadCase(**vals)


class TestSymmetric(unittest.TestCase):
    def test_symmetric_laminate_has_zero_b(self):
        lam = make_laminate([0, 45, -45, 90, 90, -45, 45, 0])
        _, b, _ = assemble_abd(lam, make_material())
        for row in b:
            for v in row:
                self.assertAlmostEqual(v, 0.0, places=6)

    def test_symmetric_pure_nx_no_curvature(self):
        lam = make_laminate([0, 90, 90, 0])
        res = solve_case(lam, make_material(), zero_case(nx=1e3))
        for k in res.curvature:
            self.assertAlmostEqual(k, 0.0, places=18)


class TestFreeThermal(unittest.TestCase):
    def test_symmetric_free_thermal_no_stress_resultants(self):
        lam = make_laminate([0, 90, 90, 0])
        res = solve_case(lam, make_material(), zero_case(delta_t=-100.0))
        for r in res.residual_forces + res.residual_moments:
            self.assertAlmostEqual(r, 0.0, places=6)
        self.assertGreater(abs(res.midplane_strain[0]), 0.0)
        for k in res.curvature:
            self.assertAlmostEqual(k, 0.0, places=15)

    def test_isotropic_like_free_strain(self):
        lam = make_laminate([0])
        mat = make_material()["t300"]
        res = solve_case(lam, make_material(), zero_case(delta_t=50.0))
        self.assertAlmostEqual(
            res.midplane_strain[0], mat.alpha1 * 50.0, places=12
        )
        self.assertAlmostEqual(
            res.midplane_strain[1], mat.alpha2 * 50.0, places=12
        )


class TestReversedLayup(unittest.TestCase):
    def test_reversed_layup_same_a_d_negated_b(self):
        mats = make_material()
        lam1 = make_laminate([0, 30, 60, 90])
        lam2 = make_laminate([90, 60, 30, 0])
        a1, b1, d1 = assemble_abd(lam1, mats)
        a2, b2, d2 = assemble_abd(lam2, mats)
        for i in range(3):
            for j in range(3):
                self.assertAlmostEqual(a1[i][j], a2[i][j], places=4)
                self.assertAlmostEqual(d1[i][j], d2[i][j], places=4)
                self.assertAlmostEqual(b1[i][j], -b2[i][j], places=4)

    def test_antisymmetric_bending_coupling(self):
        mats = make_material()
        _, b1, _ = assemble_abd(make_laminate([0, 90]), mats)
        _, b2, _ = assemble_abd(make_laminate([90, 0]), mats)
        self.assertGreater(abs(b1[0][0]), 1e3)
        self.assertAlmostEqual(b1[0][0], -b2[0][0], places=4)


class TestEquilibriumCheck(unittest.TestCase):
    def test_residuals_vanish_for_general_loading(self):
        lam = make_laminate([0, 45, 90])
        case = zero_case(nx=1e4, ny=-2e3, nxy=500.0,
                         mx=3.0, my=-1.0, mxy=0.2, delta_t=75.0)
        res = solve_case(lam, make_material(), case)
        for r in res.residual_forces:
            self.assertAlmostEqual(r, 0.0, places=6)
        for r in res.residual_moments:
            self.assertAlmostEqual(r, 0.0, places=9)

    def test_cases_are_independent(self):
        lam = make_laminate([0, 90])
        r1 = solve_case(lam, make_material(), zero_case(nx=1e3))
        r2 = solve_case(lam, make_material(), zero_case(nx=1e3))
        self.assertEqual(r1.midplane_strain, r2.midplane_strain)


class TestTransform(unittest.TestCase):
    def test_q_rotation_90_swaps_axes(self):
        mat = make_material()["t300"]
        q = mat.q_matrix()
        qb = transform_q(q, 90.0)
        self.assertAlmostEqual(qb[0][0], q[1][1], places=2)
        self.assertAlmostEqual(qb[1][1], q[0][0], places=2)
        self.assertAlmostEqual(qb[2][2], q[2][2], places=2)


if __name__ == "__main__":
    unittest.main()
