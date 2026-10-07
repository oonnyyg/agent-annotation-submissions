"""Tests for the series bar plasticity tool (stdlib unittest)."""

from __future__ import annotations

import math
import unittest

from series_bar.material import MaterialParams, MaterialState, return_mapping
from series_bar.runner import RunConfig, run_history
from series_bar.solver import BarModel, Element

E = 200_000.0
H = 10_000.0
CFG = RunConfig(tol_force=1e-8, tol_disp=1e-12, max_iter=50)


def make_element(length, area, yield_strength, hardening=H):
    return Element(
        length=length,
        area=area,
        material=MaterialParams(
            youngs_modulus=E,
            yield_strength=yield_strength,
            hardening_modulus=hardening,
        ),
    )


class ReturnMappingTests(unittest.TestCase):
    def test_elastic_predictor_inside_yield(self):
        params = MaterialParams(E, 250.0, H)
        stress, tangent, state = return_mapping(0.001, MaterialState(), params)
        self.assertAlmostEqual(stress, 200.0)
        self.assertAlmostEqual(tangent, E)
        self.assertEqual(state.eps_p, 0.0)
        self.assertEqual(state.alpha, 0.0)

    def test_plastic_correction_and_tangent(self):
        params = MaterialParams(E, 250.0, H)
        stress, tangent, state = return_mapping(0.002, MaterialState(), params)
        d_gamma = (E * 0.002 - 250.0) / (E + H)
        self.assertAlmostEqual(stress, 250.0 + H * d_gamma)
        self.assertAlmostEqual(state.eps_p, d_gamma)
        self.assertAlmostEqual(state.alpha, d_gamma)
        self.assertAlmostEqual(tangent, E * H / (E + H))

    def test_input_state_not_mutated(self):
        params = MaterialParams(E, 250.0, H)
        old = MaterialState(eps_p=0.001, alpha=0.001)
        return_mapping(0.005, old, params)
        self.assertEqual(old.eps_p, 0.001)
        self.assertEqual(old.alpha, 0.001)


class PureElasticTests(unittest.TestCase):
    def test_two_elements_different_areas(self):
        model = BarModel([
            make_element(100.0, 10.0, 1e12),
            make_element(200.0, 20.0, 1e12),
        ])
        u_end = 0.5
        results, failed = run_history(model, [u_end], CFG)
        self.assertIsNone(failed)
        res = results[0]
        compliance = 100.0 / (10.0 * E) + 200.0 / (20.0 * E)
        force = u_end / compliance
        self.assertAlmostEqual(res.reaction, -force, places=6)
        self.assertAlmostEqual(res.elements[0].stress, force / 10.0, places=6)
        self.assertAlmostEqual(res.elements[1].stress, force / 20.0, places=6)
        # Elongation is NOT split evenly: stiffer element deforms less.
        du1 = res.node_displacements[1] - res.node_displacements[0]
        du2 = res.node_displacements[2] - res.node_displacements[1]
        self.assertAlmostEqual(du1, force * 100.0 / (10.0 * E), places=9)
        self.assertAlmostEqual(du2, force * 200.0 / (20.0 * E), places=9)
        self.assertLess(res.force_imbalance, 1e-8)
        self.assertLess(res.compatibility_residual, 1e-12)
        for el in res.elements:
            self.assertEqual(el.eps_p, 0.0)
            self.assertEqual(el.alpha, 0.0)


class SequentialYieldTests(unittest.TestCase):
    def test_weaker_element_yields_first(self):
        model = BarModel([
            make_element(100.0, 10.0, 250.0),
            make_element(100.0, 10.0, 500.0),
        ])
        # Average strain 0.004; equilibrium stress 300 MPa yields only element 1.
        results, failed = run_history(model, [0.8], CFG)
        self.assertIsNone(failed)
        res = results[0]
        el1, el2 = res.elements
        # Equal areas -> equal axial force -> equal stress.
        self.assertAlmostEqual(el1.stress, el2.stress, places=6)
        self.assertAlmostEqual(el1.stress, 300.0, places=4)
        self.assertGreater(el1.eps_p, 0.0)
        # Stronger element stays elastic: solver must not smear plasticity.
        self.assertEqual(el2.eps_p, 0.0)
        self.assertEqual(el2.alpha, 0.0)
        # Stress consistent with element 1's yield surface.
        self.assertAlmostEqual(el1.stress, 250.0 + H * el1.alpha, places=4)
        self.assertLess(res.force_imbalance, 1e-8)

    def test_only_weaker_yields_at_moderate_load(self):
        model = BarModel([
            make_element(100.0, 10.0, 250.0),
            make_element(100.0, 10.0, 500.0),
        ])
        # Average strain 0.002: above yield of element 1, below element 2.
        results, failed = run_history(model, [0.4], CFG)
        self.assertIsNone(failed)
        el1, el2 = results[0].elements
        self.assertGreater(el1.eps_p, 0.0)
        self.assertEqual(el2.eps_p, 0.0)


class ReverseLoadingTests(unittest.TestCase):
    def test_tension_then_compression(self):
        model = BarModel([make_element(100.0, 10.0, 250.0)])
        history = [0.4, 0.3, -0.4, 0.0]
        results, failed = run_history(model, history, CFG)
        self.assertIsNone(failed)
        r_t, r_unload, r_c, r_zero = results

        # After tensile plasticity, positive signed plastic strain remains.
        self.assertGreater(r_t.elements[0].eps_p, 0.0)
        alpha_after_tension = r_t.elements[0].alpha

        # Partial unloading stays elastic: alpha unchanged, stress drops.
        self.assertAlmostEqual(r_unload.elements[0].alpha, alpha_after_tension)
        self.assertAlmostEqual(r_unload.elements[0].eps_p, r_t.elements[0].eps_p)
        self.assertAlmostEqual(
            r_unload.elements[0].stress,
            E * (0.003 - r_unload.elements[0].eps_p),
            places=4,
        )

        # Reverse yield: isotropic hardening, compression yield at -(sy0+H*alpha).
        self.assertLess(r_c.elements[0].eps_p, r_t.elements[0].eps_p)
        self.assertGreater(r_c.elements[0].alpha, alpha_after_tension)
        self.assertAlmostEqual(
            r_c.elements[0].stress,
            -(250.0 + H * r_c.elements[0].alpha),
            places=4,
        )

        # Final return to u=0: signed plastic strain is a true residual,
        # not solver noise; alpha is monotonically non-decreasing.
        self.assertNotAlmostEqual(r_zero.elements[0].eps_p, 0.0, places=6)
        alphas = [r.elements[0].alpha for r in results]
        self.assertEqual(alphas, sorted(alphas))


class ValidationTests(unittest.TestCase):
    def test_rejects_nonpositive_area(self):
        with self.assertRaises(ValueError):
            BarModel([make_element(100.0, 0.0, 250.0)])

    def test_rejects_nonfinite_length(self):
        with self.assertRaises(ValueError):
            BarModel([make_element(math.inf, 10.0, 250.0)])

    def test_rejects_nonpositive_hardening(self):
        with self.assertRaises(ValueError):
            BarModel([make_element(100.0, 10.0, 250.0, hardening=0.0)])

    def test_rejects_nonfinite_displacement(self):
        model = BarModel([make_element(100.0, 10.0, 250.0)])
        with self.assertRaises(ValueError):
            model.solve_step(1, math.nan, 1e-8, 1e-12, 50)


class FailurePathTests(unittest.TestCase):
    def test_nonconverged_step_marks_failure_and_keeps_history(self):
        model = BarModel([
            make_element(100.0, 10.0, 250.0),
            make_element(100.0, 10.0, 250.0),
        ])
        # First step converges elastically.
        ok = model.solve_step(1, 0.1, 1e-8, 1e-12, 50)
        self.assertTrue(ok.converged)
        # Force failure with max_iter=1 on a plastic step.
        bad = model.solve_step(2, 0.4, 1e-8, 1e-12, 1)
        self.assertFalse(bad.converged)
        self.assertEqual(bad.node_displacements, [])
        self.assertEqual(bad.elements, [])
        # Material history remains at the last converged state.
        self.assertEqual(model.states[0].eps_p, 0.0)
        self.assertEqual(model.states[0].alpha, 0.0)
        # Model still usable afterwards.
        good = model.solve_step(3, 0.4, 1e-8, 1e-12, 50)
        self.assertTrue(good.converged)


if __name__ == "__main__":
    unittest.main()
