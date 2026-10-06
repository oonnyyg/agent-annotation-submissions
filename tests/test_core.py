import math
import unittest

from allele_drift.core import (
    ConfigError,
    Scenario,
    binomial_row,
    run_scenario,
    validate_config,
)


def neutral(name="neutral", n=10, i0=4, g=20, s=0.0, u=0.0, v=0.0):
    return Scenario(name, n, i0, g, s, u, v)


class NeutralCase(unittest.TestCase):
    def test_expected_count_is_preserved(self):
        result = run_scenario(neutral())
        for row in result["generations"]:
            expected_count = row["expected_frequency"] * 10
            self.assertAlmostEqual(expected_count, 4.0, places=12)

    def test_probability_sums_to_one(self):
        result = run_scenario(neutral())
        for row in result["generations"]:
            self.assertAlmostEqual(row["probability_sum"], 1.0, places=12)

    def test_deterministic_start(self):
        result = run_scenario(neutral())
        dist = result["generations"][0]["distribution"]
        self.assertEqual(dist[4], 1.0)
        self.assertEqual(sum(dist), 1.0)

    def test_heterozygosity_decays_from_deterministic_start(self):
        result = run_scenario(neutral())
        gens = result["generations"]
        self.assertAlmostEqual(
            gens[0]["expected_heterozygosity"], 2 * 0.4 * 0.6, places=12)
        self.assertLess(gens[-1]["expected_heterozygosity"],
                        gens[0]["expected_heterozygosity"])


class OneWayMutation(unittest.TestCase):
    def test_matches_binomial_after_one_step(self):
        # Only normal -> target mutation (u > 0, v = 0): from count 0 the
        # next generation is exactly Binomial(N, u).
        result = run_scenario(neutral(n=6, i0=0, g=1, u=0.3))
        dist = result["generations"][1]["distribution"]
        for k in range(7):
            expected = math.comb(6, k) * 0.3**k * 0.7**(6 - k)
            self.assertAlmostEqual(dist[k], expected, places=12)

    def test_expectation_increases_monotonically(self):
        result = run_scenario(neutral(n=6, i0=0, g=8, u=0.3))
        freqs = [row["expected_frequency"] for row in result["generations"]]
        for earlier, later in zip(freqs, freqs[1:]):
            self.assertLess(earlier, later)

    def test_boundary_not_absorbing_with_mutation(self):
        # Start at 0 with u > 0: the population leaves the boundary, so
        # the distribution at 0 must become zero while the first-hit
        # cumulative stays at 1 (hit recorded once, at generation 0).
        result = run_scenario(neutral(n=6, i0=0, g=5, u=0.3))
        gens = result["generations"]
        self.assertEqual(gens[0]["first_hit_increment"], 1.0)
        for row in gens:
            self.assertEqual(row["first_hit_cumulative"], 1.0)
            self.assertEqual(row["untouched_probability"], 0.0)
        self.assertGreater(gens[1]["distribution"][0], 0.0)


class InitialBoundary(unittest.TestCase):
    def test_hit_recorded_at_generation_zero(self):
        result = run_scenario(neutral(n=8, i0=8, g=10))
        gens = result["generations"]
        self.assertEqual(gens[0]["first_hit_increment"], 1.0)
        self.assertEqual(gens[0]["first_hit_cumulative"], 1.0)
        for row in gens[1:]:
            self.assertEqual(row["first_hit_increment"], 0.0)

    def test_interior_start_eventually_hits(self):
        # Neutral, no mutation: boundaries are absorbing here, so the
        # cumulative hit probability must approach 1.
        result = run_scenario(neutral(n=6, i0=3, g=200))
        last = result["generations"][-1]
        self.assertGreater(last["first_hit_cumulative"], 0.99)
        self.assertLess(last["untouched_probability"], 0.01)

    def test_no_double_counting_after_leaving_boundary(self):
        # Start at 0 with mutation both ways: increments after
        # generation 0 must all be zero because the hit was already
        # recorded, even though the process revisits 0 with positive
        # probability.
        result = run_scenario(neutral(n=4, i0=0, g=10, u=0.4, v=0.4))
        gens = result["generations"]
        self.assertTrue(any(row["distribution"][0] > 0.0 for row in gens[1:]))
        for row in gens[1:]:
            self.assertEqual(row["first_hit_increment"], 0.0)


class BinomialRow(unittest.TestCase):
    def test_extreme_probabilities(self):
        self.assertEqual(binomial_row(5, 0.0), [1.0, 0.0, 0.0, 0.0, 0.0, 0.0])
        self.assertEqual(binomial_row(5, 1.0), [0.0, 0.0, 0.0, 0.0, 0.0, 1.0])

    def test_row_sums_to_one(self):
        row = binomial_row(30, 0.37)
        self.assertAlmostEqual(sum(row), 1.0, places=12)
        self.assertTrue(all(p >= 0.0 for p in row))


class Validation(unittest.TestCase):
    def base(self, **overrides):
        raw = {
            "population_size": 10,
            "initial_count": 3,
            "generations": 5,
            "selection_coefficient": 0.1,
            "mutation_to_target": 0.01,
            "mutation_from_target": 0.02,
        }
        raw.update(overrides)
        return raw

    def test_valid_config(self):
        scenario = validate_config(self.base(name="ok"))
        self.assertEqual(scenario.name, "ok")

    def test_rejects_nonpositive_target_fitness(self):
        with self.assertRaises(ConfigError):
            validate_config(self.base(selection_coefficient=-1.0))
        with self.assertRaises(ConfigError):
            validate_config(self.base(selection_coefficient=-1.5))

    def test_rejects_bad_counts_and_generations(self):
        for bad in (0, -3, 2.5, True):
            with self.assertRaises(ConfigError):
                validate_config(self.base(population_size=bad))
        with self.assertRaises(ConfigError):
            validate_config(self.base(initial_count=11))
        with self.assertRaises(ConfigError):
            validate_config(self.base(generations=-1))

    def test_rejects_bad_mutation_probabilities(self):
        for bad in (-0.1, 1.1, float("nan")):
            with self.assertRaises(ConfigError):
                validate_config(self.base(mutation_to_target=bad))
            with self.assertRaises(ConfigError):
                validate_config(self.base(mutation_from_target=bad))

    def test_rejects_missing_field(self):
        raw = self.base()
        del raw["generations"]
        with self.assertRaises(ConfigError):
            validate_config(raw)


if __name__ == "__main__":
    unittest.main()
