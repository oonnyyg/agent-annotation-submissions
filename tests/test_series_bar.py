"""串联杆件循环塑性分析的单元测试（标准库 unittest）。"""

from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path
from unittest import mock

from series_bar.material import MaterialPointResult, MaterialState, return_mapping
from series_bar.model import BarModel, Material, ModelError, Segment, load_model
from series_bar.report import LoadHistoryError, load_displacement_history
from series_bar.solver import SolverError, run_analysis

E = 200_000.0
SY = 250.0
H = 2_000.0


def make_model(areas=(100.0, 200.0), lengths=(1.0, 1.0), yields=(SY, SY)) -> BarModel:
    segments = tuple(
        Segment(length=length, area=area, material=Material(E, sy, H))
        for length, area, sy in zip(lengths, areas, yields)
    )
    return BarModel(segments)


class ElasticTests(unittest.TestCase):
    def test_pure_elastic_force_and_elongation_split(self):
        model = make_model(areas=(100.0, 200.0), lengths=(1.0, 2.0))
        result = run_analysis(model, [1e-4])
        self.assertTrue(result.completed)
        step = result.steps[0]
        flexibility = 1.0 / (E * 100.0) + 2.0 / (E * 200.0)
        expected_force = 1e-4 / flexibility
        self.assertAlmostEqual(step.axial_force, expected_force, places=6)
        self.assertAlmostEqual(step.reaction_force, -expected_force, places=6)
        # 各段轴力相同，但应力与伸长不同（截面积、长度不同）
        forces = [seg.axial_force for seg in step.segments]
        self.assertAlmostEqual(forces[0], forces[1], places=6)
        stresses = [seg.stress for seg in step.segments]
        self.assertAlmostEqual(stresses[0], 2.0 * stresses[1], places=6)
        elong1 = step.node_displacements[1] - step.node_displacements[0]
        elong2 = step.node_displacements[2] - step.node_displacements[1]
        self.assertAlmostEqual(elong1, expected_force / (E * 100.0), places=9)
        self.assertAlmostEqual(elong2, 2.0 * expected_force / (E * 200.0), places=9)
        self.assertLess(step.force_imbalance, 1e-6)
        self.assertLess(step.compatibility_residual, 1e-12)
        self.assertTrue(all(not seg.yielded for seg in step.segments))


class PlasticTests(unittest.TestCase):
    def test_sequential_yielding(self):
        model = make_model(areas=(100.0, 100.0), yields=(200.0, 300.0))
        # 位移足够大时两段先后屈服
        result = run_analysis(model, [0.001, 0.01, 0.06])
        self.assertTrue(result.completed)
        first, _, last = result.steps
        self.assertFalse(any(seg.yielded for seg in first.segments))
        self.assertTrue(all(seg.yielded for seg in last.segments))
        # 弱段塑性应变更大
        weak, strong = last.segments
        self.assertGreater(weak.plastic_strain, strong.plastic_strain)
        # 轴力相等而非应力相等（本例面积相同，应力也相等）
        self.assertAlmostEqual(weak.axial_force, strong.axial_force, places=6)
        # 累计塑性应变单调不减
        accum = [
            [seg.accumulated_plastic_strain for seg in step.segments]
            for step in result.steps
        ]
        for prev, curr in zip(accum, accum[1:]):
            for a, b in zip(prev, curr):
                self.assertGreaterEqual(b, a)

    def test_reverse_loading(self):
        model = make_model(areas=(100.0, 100.0), lengths=(1.0, 1.0))
        result = run_analysis(model, [0.01, 0.005, -0.005, -0.02])
        self.assertTrue(result.completed)
        tension = result.steps[0]
        unload = result.steps[1]
        reverse = result.steps[3]
        # 拉伸屈服产生正塑性应变
        self.assertGreater(tension.segments[0].plastic_strain, 0.0)
        # 卸载为弹性：塑性应变不变
        self.assertAlmostEqual(
            unload.segments[0].plastic_strain,
            tension.segments[0].plastic_strain,
            places=12,
        )
        # 反向加载后塑性应变变号，累计量只增不减
        self.assertLess(reverse.segments[0].plastic_strain, 0.0)
        self.assertGreater(
            reverse.segments[0].accumulated_plastic_strain,
            tension.segments[0].accumulated_plastic_strain,
        )
        # 各向同性硬化：反向屈服应力量值等于拉伸侧当前屈服强度
        alpha = tension.segments[0].accumulated_plastic_strain
        self.assertGreater(abs(reverse.segments[0].stress), SY + H * alpha - 1e-6)

    def test_residual_deformation_after_unload_to_zero(self):
        model = make_model(areas=(100.0,), lengths=(1.0,))
        result = run_analysis(model, [0.01, 0.009, 0.008])
        self.assertTrue(result.completed)
        loaded = result.steps[0]
        final = result.steps[-1]
        # 弹性卸载：塑性应变保留（真实残余变形），应力按弹性斜率下降
        self.assertAlmostEqual(
            final.segments[0].plastic_strain,
            loaded.segments[0].plastic_strain,
            places=12,
        )
        self.assertGreater(final.segments[0].plastic_strain, 0.0)
        expected_stress = E * (0.008 - loaded.segments[0].plastic_strain)
        self.assertAlmostEqual(final.segments[0].stress, expected_stress, places=6)
        self.assertLess(final.force_imbalance, 1e-6)


class FailureHandlingTests(unittest.TestCase):
    def test_failed_step_keeps_previous_results(self):
        model = make_model(areas=(100.0, 200.0))
        fake_point = MaterialPointResult(1.0, E, MaterialState(), False)
        with mock.patch(
            "series_bar.solver.return_mapping", return_value=fake_point
        ):
            result = run_analysis(model, [0.001, 0.002, 0.003], max_iterations=5)
        self.assertFalse(result.completed)
        self.assertEqual(result.failed_step, 0)
        failed = result.steps[-1]
        self.assertFalse(failed.converged)
        # 失败步不补零：结果字段为 None
        self.assertIsNone(failed.axial_force)
        self.assertIsNone(failed.node_displacements)
        self.assertIsNone(failed.segments)

    def test_nonfinite_displacement_rejected(self):
        model = make_model()
        with self.assertRaises(SolverError):
            run_analysis(model, [0.1, float("nan")])
        with self.assertRaises(SolverError):
            run_analysis(model, [float("inf")])


class InputValidationTests(unittest.TestCase):
    def write_json(self, payload) -> Path:
        handle = tempfile.NamedTemporaryFile(
            "w", suffix=".json", delete=False, encoding="utf-8"
        )
        json.dump(payload, handle)
        handle.close()
        return Path(handle.name)

    def test_invalid_model_parameters(self):
        base = {
            "segments": [{"length": 1.0, "area": 100.0}],
            "material": {
                "elastic_modulus": E,
                "yield_strength": SY,
                "hardening_modulus": H,
            },
        }
        good = load_model(self.write_json(base))
        self.assertEqual(len(good.segments), 1)

        bad_cases = [
            {"segments": []},
            {"segments": [{"length": -1.0, "area": 100.0}], "material": base["material"]},
            {"segments": [{"length": 1.0, "area": 0.0}], "material": base["material"]},
            {
                "segments": [{"length": 1.0, "area": 100.0}],
                "material": {
                    "elastic_modulus": float("nan"),
                    "yield_strength": SY,
                    "hardening_modulus": H,
                },
            },
            {
                "segments": [{"length": 1.0, "area": 100.0}],
                "material": {
                    "elastic_modulus": E,
                    "yield_strength": SY,
                    "hardening_modulus": -1.0,
                },
            },
            {"segments": [{"length": 1.0, "area": 100.0}]},
        ]
        for payload in bad_cases:
            with self.assertRaises(ModelError, msg=str(payload)):
                load_model(self.write_json(payload))

    def test_displacement_csv_parsing(self):
        with tempfile.TemporaryDirectory() as tmp:
            plain = Path(tmp) / "plain.csv"
            plain.write_text("0.0\n0.1\n-0.2\n", encoding="utf-8")
            self.assertEqual(load_displacement_history(plain), [0.0, 0.1, -0.2])

            headed = Path(tmp) / "headed.csv"
            headed.write_text("step,displacement\n0,0.0\n1,0.3\n", encoding="utf-8")
            self.assertEqual(load_displacement_history(headed), [0.0, 0.3])

            bad = Path(tmp) / "bad.csv"
            bad.write_text("displacement\nabc\n", encoding="utf-8")
            with self.assertRaises(LoadHistoryError):
                load_displacement_history(bad)

            nan_file = Path(tmp) / "nan.csv"
            nan_file.write_text("0.0\nnan\n", encoding="utf-8")
            with self.assertRaises(LoadHistoryError):
                load_displacement_history(nan_file)


class ReturnMappingTests(unittest.TestCase):
    def test_trial_state_does_not_mutate_history(self):
        material = Material(E, SY, H)
        state = MaterialState(0.001, 0.002)
        result = return_mapping(material, state, 0.01)
        self.assertTrue(result.yielded)
        # 原历史对象不被修改，试算状态独立
        self.assertEqual(state, MaterialState(0.001, 0.002))
        self.assertIsNot(result.state, state)
        # 重复试算结果一致，不会重复累加塑性
        again = return_mapping(material, state, 0.01)
        self.assertEqual(result.state, again.state)

    def test_accumulated_never_decreases(self):
        material = Material(E, SY, H)
        state = MaterialState()
        state = return_mapping(material, state, 0.01).state
        alpha_tension = state.accumulated_plastic_strain
        state = return_mapping(material, state, -0.01).state
        self.assertGreater(state.accumulated_plastic_strain, alpha_tension)
        self.assertLess(state.plastic_strain, 0.0)


if __name__ == "__main__":
    unittest.main()
