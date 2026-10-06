"""Tests for damage accumulation and the JSON summary."""
from __future__ import annotations

import unittest

from rainflow_audit.damage import SNModel, evaluate_cycles, summarize
from rainflow_audit.rainflow import Cycle


def _cycle(stress_range: float, mean: float, count: float) -> Cycle:
    return Cycle(stress_range, mean, count, 0.0, 1.0)


class TestDamage(unittest.TestCase):
    def test_no_mean_correction(self):
        model = SNModel(reference_stress=100.0, exponent=5.0)
        cycles = [_cycle(10.0, 0.0, 0.5), _cycle(10.0, 0.0, 1.0), _cycle(20.0, 0.0, 0.5)]
        results = evaluate_cycles(cycles, model)
        self.assertTrue(all(r.evaluable for r in results))
        life_amp5 = (100.0 / 5.0) ** 5
        life_amp10 = (100.0 / 10.0) ** 5
        expected = 1.5 / life_amp5 + 0.5 / life_amp10
        self.assertAlmostEqual(summarize(results)["total_damage"], expected)

    def test_zero_range_cycle_no_damage(self):
        model = SNModel(reference_stress=100.0, exponent=5.0)
        results = evaluate_cycles([_cycle(0.0, 3.0, 0.5)], model)
        self.assertEqual(results[0].damage, 0.0)
        self.assertFalse(results[0].evaluable)
        summary = summarize(results)
        self.assertEqual(summary["total_damage"], 0.0)
        self.assertTrue(summary["damage_complete"])

    def test_goodman_correction(self):
        model = SNModel(reference_stress=100.0, exponent=5.0,
                        mean_correction="goodman", ultimate_strength=200.0)
        results = evaluate_cycles([_cycle(10.0, 40.0, 1.0)], model)
        denominator = 1.0 - 40.0 / 200.0
        equivalent = 5.0 / denominator
        self.assertAlmostEqual(results[0].equivalent_amplitude, equivalent)
        self.assertAlmostEqual(results[0].life, (100.0 / equivalent) ** 5)

    def test_goodman_negative_mean_ignored(self):
        model = SNModel(reference_stress=100.0, exponent=5.0,
                        mean_correction="goodman", ultimate_strength=200.0)
        results = evaluate_cycles([_cycle(10.0, -50.0, 1.0)], model)
        self.assertAlmostEqual(results[0].equivalent_amplitude, 5.0)

    def test_goodman_inevaluable_not_masked(self):
        model = SNModel(reference_stress=100.0, exponent=5.0,
                        mean_correction="goodman", ultimate_strength=200.0)
        cycles = [_cycle(10.0, 250.0, 1.0), _cycle(10.0, 0.0, 1.0)]
        results = evaluate_cycles(cycles, model)
        self.assertFalse(results[0].evaluable)
        self.assertEqual(results[0].damage, 0.0)
        summary = summarize(results)
        self.assertFalse(summary["damage_complete"])
        self.assertIsNotNone(summary["warning"])
        self.assertEqual(summary["cycles_inevaluable"], 1)

    def test_model_validation(self):
        with self.assertRaises(ValueError):
            SNModel(reference_stress=-1.0, exponent=5.0)
        with self.assertRaises(ValueError):
            SNModel(reference_stress=100.0, exponent=0.0)
        with self.assertRaises(ValueError):
            SNModel(reference_stress=100.0, exponent=5.0, mean_correction="goodman")


if __name__ == "__main__":
    unittest.main()
