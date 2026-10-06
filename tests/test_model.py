import math
import unittest

from allele_drift.model import run_scenario


def base(**overrides):
    cfg = {
        "name": "t",
        "population_size": 10,
        "initial_count": 5,
        "generations": 20,
        "selection_coefficient": 0.0,
        "mutation_to_target": 0.0,
        "mutation_to_ordinary": 0.0,
    }
    cfg.update(overrides)
    return cfg


class NeutralCase(unittest.TestCase):
    def test_expected_frequency_stays_at_initial(self):
        result = run_scenario(base())
        for row in result.rows:
            self.assertAlmostEqual(row.expected_frequency, 0.5, places=12)

    def test_probability_sums_to_one(self):
        result = run_scenario(base(generations=50))
        self.assertLess(result.max_sum_error, 1e-12)

    def test_no_mutation_absorbs_at_boundaries(self):
        result = run_scenario(base(generations=200))
        last = result.rows[-1]
        self.assertAlmostEqual(last.prob_untouched_boundary, 0.0, places=6)
        self.assertAlmostEqual(last.boundary_hit_cumulative, 1.0, places=6)
        self.assertAlmostEqual(last.probabilities[10], 0.5, places=6)


class OneWayMutation(unittest.TestCase):
    def test_all_mass_moves_to_target(self):
        result = run_scenario(base(mutation_to_target=0.1, generations=300))
        last = result.rows[-1]
        self.assertAlmostEqual(last.probabilities[10], 1.0, places=6)
        self.assertAlmostEqual(last.expected_frequency, 1.0, places=6)

    def test_boundary_not_absorbing_with_mutation(self):
        result = run_scenario(base(initial_count=0, mutation_to_target=0.2, generations=5))
        self.assertEqual(result.rows[0].boundary_hit_cumulative, 1.0)
        self.assertGreater(result.rows[5].probabilities[1], 0.0)


class InitialBoundary(unittest.TestCase):
    def test_hit_recorded_at_generation_zero(self):
        result = run_scenario(base(initial_count=10, generations=10))
        self.assertEqual(result.rows[0].boundary_hit_increment, 1.0)
        self.assertEqual(result.rows[0].prob_untouched_boundary, 0.0)
        for row in result.rows[1:]:
            self.assertEqual(row.boundary_hit_increment, 0.0)
            self.assertEqual(row.boundary_hit_cumulative, 1.0)

    def test_no_double_count_after_leaving_and_returning(self):
        result = run_scenario(base(initial_count=0, mutation_to_target=0.5,
                                   mutation_to_ordinary=0.5, generations=30))
        for row in result.rows[1:]:
            self.assertEqual(row.boundary_hit_increment, 0.0)


class Validation(unittest.TestCase):
    def test_rejects_nonpositive_fitness(self):
        with self.assertRaises(ValueError):
            run_scenario(base(selection_coefficient=-1.0))

    def test_rejects_bad_mutation_rate(self):
        with self.assertRaises(ValueError):
            run_scenario(base(mutation_to_target=1.5))

    def test_rejects_bad_initial_count(self):
        with self.assertRaises(ValueError):
            run_scenario(base(initial_count=11))


class Heterozygosity(unittest.TestCase):
    def test_expected_heterozygosity_formula(self):
        result = run_scenario(base(generations=1))
        row = result.rows[1]
        n = 10
        manual = sum(
            2.0 * (j / n) * (1.0 - j / n) * row.probabilities[j] for j in range(n + 1)
        )
        self.assertTrue(math.isclose(row.expected_heterozygosity, manual))


if __name__ == "__main__":
    unittest.main()
