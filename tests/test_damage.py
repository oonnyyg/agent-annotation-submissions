import unittest

from rainflow_fatigue.damage import DamageModel, assess_cycles, total_damage
from rainflow_fatigue.rainflow import Cycle


def cycle(rng, mean, count=1.0):
    return Cycle(0.0, 1.0, rng, mean, count)


class DamageTest(unittest.TestCase):
    def test_no_correction_life_and_damage(self):
        model = DamageModel(reference_stress=100.0, exponent=3.0)
        (result,) = assess_cycles([cycle(40.0, 0.0)], model)
        self.assertAlmostEqual(result.life, (100.0 / 20.0) ** 3)
        self.assertAlmostEqual(result.damage, 1.0 / result.life)

    def test_goodman_correction_uses_positive_mean_only(self):
        model = DamageModel(100.0, 2.0, "goodman", ultimate_strength=400.0)
        pos, neg = assess_cycles([cycle(40.0, 100.0), cycle(40.0, -100.0)], model)
        self.assertAlmostEqual(pos.equivalent_amplitude, 20.0 / (1.0 - 0.25))
        self.assertAlmostEqual(neg.equivalent_amplitude, 20.0)

    def test_nonpositive_denominator_marks_not_evaluable(self):
        model = DamageModel(100.0, 2.0, "goodman", ultimate_strength=100.0)
        results = assess_cycles([cycle(40.0, 200.0), cycle(20.0, 0.0)], model)
        self.assertFalse(results[0].evaluable)
        self.assertIsNone(results[0].damage)
        self.assertIsNone(total_damage(results))

    def test_zero_range_cycle_contributes_nothing(self):
        model = DamageModel(100.0, 3.0)
        (result,) = assess_cycles([cycle(0.0, 50.0, 0.5)], model)
        self.assertTrue(result.evaluable)
        self.assertEqual(result.damage, 0.0)

    def test_half_cycle_counts_half_damage(self):
        model = DamageModel(100.0, 2.0)
        half, full = assess_cycles(
            [cycle(40.0, 0.0, 0.5), cycle(40.0, 0.0, 1.0)], model)
        self.assertAlmostEqual(half.damage * 2.0, full.damage)

    def test_invalid_model_rejected(self):
        with self.assertRaises(ValueError):
            DamageModel(-1.0, 2.0)
        with self.assertRaises(ValueError):
            DamageModel(100.0, 0.0)
        with self.assertRaises(ValueError):
            DamageModel(100.0, 2.0, "goodman")


if __name__ == "__main__":
    unittest.main()
