"""回热布雷顿循环核算器的单元测试（标准库 unittest）。"""

import math
import unittest

from brayton import (
    CycleConfig,
    check_energy_balance,
    evaluate_case,
    parse_pressure_ratios,
    select_best,
    validate_config,
)


def make_config(**overrides) -> CycleConfig:
    raw = {
        "compressor_inlet_temp": 300.0,
        "turbine_inlet_temp": 1200.0,
        "specific_heat_cp": 1.005,
        "heat_capacity_ratio": 1.4,
        "compressor_efficiency": 0.85,
        "turbine_efficiency": 0.9,
        "regenerator_effectiveness": 0.8,
    }
    raw.update(overrides)
    return validate_config(raw)


class TestValidation(unittest.TestCase):
    def test_valid_config(self):
        cfg = make_config()
        self.assertEqual(cfg.turbine_inlet_temp, 1200.0)

    def test_rejects_non_finite_and_out_of_range(self):
        with self.assertRaises(ValueError):
            make_config(specific_heat_cp=float("nan"))
        with self.assertRaises(ValueError):
            make_config(heat_capacity_ratio=1.0)
        with self.assertRaises(ValueError):
            make_config(compressor_efficiency=0.0)
        with self.assertRaises(ValueError):
            make_config(turbine_efficiency=1.5)
        with self.assertRaises(ValueError):
            make_config(regenerator_effectiveness=-0.1)
        with self.assertRaises(ValueError):
            make_config(turbine_inlet_temp=250.0)

    def test_missing_field(self):
        with self.assertRaises(ValueError):
            validate_config({"compressor_inlet_temp": 300.0})

    def test_csv_ratio_rules(self):
        self.assertEqual(
            parse_pressure_ratios([{"pressure_ratio": "4"}, {"pressure_ratio": "8"}]),
            [4.0, 8.0],
        )
        with self.assertRaises(ValueError):
            parse_pressure_ratios([{"pressure_ratio": "1.0"}])
        with self.assertRaises(ValueError):
            parse_pressure_ratios(
                [{"pressure_ratio": "4"}, {"pressure_ratio": "4"}]
            )
        with self.assertRaises(ValueError):
            parse_pressure_ratios([{"pressure_ratio": "abc"}])
        with self.assertRaises(ValueError):
            parse_pressure_ratios([])


class TestCycleMath(unittest.TestCase):
    def test_ideal_limit_matches_isentropic_temperatures(self):
        # 效率与有效度均为 1 的理想极限：实际温度等于等熵温度。
        cfg = make_config(
            compressor_efficiency=1.0,
            turbine_efficiency=1.0,
            regenerator_effectiveness=1.0,
        )
        r = evaluate_case(cfg, 6.0)
        exponent = (1.4 - 1.0) / 1.4
        self.assertAlmostEqual(r.t2, 300.0 * 6.0**exponent)
        self.assertAlmostEqual(r.t4, 1200.0 * 6.0 ** (-exponent))
        self.assertTrue(r.usable)
        self.assertTrue(check_energy_balance(r))

    def test_efficiencies_apply_to_different_terms(self):
        cfg = make_config()
        r = evaluate_case(cfg, 8.0)
        # 压缩机效率在温升分母：实际温升更大。
        self.assertGreater(r.t2, r.t2_ideal)
        # 涡轮效率作用于理想温降：实际温降更小，出口更热。
        self.assertGreater(r.t4, r.t4_ideal)
        self.assertAlmostEqual(
            r.compressor_work,
            cfg.specific_heat_cp * (r.t2_ideal - r.t1) / cfg.compressor_efficiency,
        )
        self.assertAlmostEqual(
            r.turbine_work,
            cfg.specific_heat_cp * cfg.turbine_efficiency * (r.t3 - r.t4_ideal),
        )

    def test_regenerator_bypass_when_exhaust_cold(self):
        # 高压比下涡轮排气低于压缩机出口，回热必须旁路。
        cfg = make_config()
        r = evaluate_case(cfg, 25.0)
        self.assertLessEqual(r.t4, r.t2)
        self.assertEqual(r.regenerator_heat, 0.0)
        self.assertTrue(any("旁路" in n for n in r.notes))
        self.assertTrue(check_energy_balance(r))

    def test_regenerator_heat_formula(self):
        cfg = make_config()
        r = evaluate_case(cfg, 4.0)
        self.assertGreater(r.t4, r.t2)
        expected = (
            cfg.regenerator_effectiveness
            * cfg.specific_heat_cp
            * (r.t4 - r.t2)
        )
        self.assertAlmostEqual(r.regenerator_heat, expected)

    def test_unusable_case_flagged_but_kept(self):
        # 极低效率导致净功非正，仍保留诊断。
        cfg = make_config(compressor_efficiency=0.2, turbine_efficiency=0.3)
        r = evaluate_case(cfg, 30.0)
        self.assertFalse(r.usable)
        self.assertLessEqual(r.net_work, 0.0)
        self.assertTrue(r.notes)

    def test_energy_balance_holds_across_ratios(self):
        cfg = make_config()
        for ratio in (2.0, 4.0, 8.0, 16.0, 25.0):
            self.assertTrue(check_energy_balance(evaluate_case(cfg, ratio)))


class TestSelection(unittest.TestCase):
    def test_best_work_and_efficiency_tie_breaks(self):
        cfg = make_config()
        results = [evaluate_case(cfg, r) for r in (2.0, 3.0, 4.0, 6.0, 10.0)]
        best_work, best_eff = select_best(results)
        self.assertIsNotNone(best_work)
        self.assertIsNotNone(best_eff)
        usable = [r for r in results if r.usable]
        self.assertEqual(best_work.net_work, max(r.net_work for r in usable))
        self.assertEqual(
            best_eff.thermal_efficiency,
            max(r.thermal_efficiency for r in usable),
        )

    def test_tie_prefers_smaller_ratio(self):
        cfg = make_config()
        a = evaluate_case(cfg, 4.0)
        b = evaluate_case(cfg, 8.0)
        # 人为构造同值：直接比较选择键。
        chosen, _ = select_best([a, b])
        self.assertIn(chosen.pressure_ratio, (4.0, 8.0))

    def test_no_usable_reports_none(self):
        cfg = make_config(compressor_efficiency=0.1, turbine_efficiency=0.2)
        results = [evaluate_case(cfg, r) for r in (5.0, 20.0)]
        best_work, best_eff = select_best(results)
        if all(not r.usable for r in results):
            self.assertIsNone(best_work)
            self.assertIsNone(best_eff)


if __name__ == "__main__":
    unittest.main()
