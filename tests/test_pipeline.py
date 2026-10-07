import unittest

from ranked_set_enrichment.core import FeatureScore, FeatureSet
from ranked_set_enrichment.pipeline import analyze
from ranked_set_enrichment.stats import benjamini_hochberg, empirical_p_value


def make_ranked(n=20):
    return [FeatureScore(f"g{i}", 10.0 - i) for i in range(n)]


class AnalyzeTest(unittest.TestCase):
    def test_enriched_set_scores_high(self):
        ranked = make_ranked()
        sets = [FeatureSet("top", ["g0", "g1", "g2", "g3"])]
        (result,) = analyze(ranked, sets, permutations=100, seed=7)
        self.assertEqual(result.status, "ok")
        self.assertGreater(result.enrichment_score, 0.5)
        self.assertIsNotNone(result.p_value)
        self.assertIsNotNone(result.q_value)
        self.assertEqual(len(result.running_sum), len(ranked))

    def test_duplicates_deduped_and_missing_reported(self):
        ranked = make_ranked()
        sets = [FeatureSet("s", ["g0", "g0", "g1", "nope"])]
        (result,) = analyze(ranked, sets, permutations=10, seed=1)
        self.assertEqual(result.member_count, 2)
        self.assertEqual(result.ignored_members, ["nope"])

    def test_empty_after_filtering_not_computable(self):
        ranked = make_ranked()
        sets = [FeatureSet("s", ["x", "y"])]
        (result,) = analyze(ranked, sets, permutations=10, seed=1)
        self.assertEqual(result.status, "not_computable")
        self.assertIsNone(result.p_value)

    def test_full_coverage_not_computable(self):
        ranked = make_ranked(5)
        sets = [FeatureSet("s", [f"g{i}" for i in range(5)])]
        (result,) = analyze(ranked, sets, permutations=10, seed=1)
        self.assertEqual(result.status, "not_computable")

    def test_member_order_does_not_change_result(self):
        ranked = make_ranked()
        members = ["g0", "g5", "g2", "g9"]
        (a,) = analyze(ranked, [FeatureSet("s", members)], 200, seed=3)
        (b,) = analyze(ranked, [FeatureSet("s", list(reversed(members)))], 200, seed=3)
        self.assertEqual(a.enrichment_score, b.enrichment_score)
        self.assertEqual(a.p_value, b.p_value)

    def test_seed_reproducibility(self):
        ranked = make_ranked()
        sets = [FeatureSet("s", ["g0", "g1", "g2"])]
        (a,) = analyze(ranked, sets, 200, seed=42)
        (b,) = analyze(ranked, sets, 200, seed=42)
        self.assertEqual(a.p_value, b.p_value)


class StatsTest(unittest.TestCase):
    def test_p_value_never_zero(self):
        self.assertEqual(empirical_p_value(1.0, [0.0] * 100), 1 / 101)

    def test_p_value_two_sided(self):
        null = [-0.9, 0.5, 0.1, -0.2]
        self.assertAlmostEqual(empirical_p_value(0.8, null), (1 + 1) / 5)

    def test_bh_monotone(self):
        q = benjamini_hochberg([0.01, 0.04, 0.5])
        self.assertAlmostEqual(q[0], 0.03)
        self.assertAlmostEqual(q[1], 0.06)
        self.assertAlmostEqual(q[2], 0.5)


if __name__ == "__main__":
    unittest.main()
