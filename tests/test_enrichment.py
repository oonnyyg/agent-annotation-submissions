import unittest

from ranked_set_enrichment.enrichment import (
    RankedFeature,
    bh_adjust,
    compute_enrichment,
    permutation_pvalue,
    prepare_set,
    running_enrichment,
    sort_ranked_list,
)


class SortAndPrepareTests(unittest.TestCase):
    def test_sort_descending_ties_by_identifier(self):
        features = [
            RankedFeature("b", 1.0),
            RankedFeature("a", 1.0),
            RankedFeature("c", 2.0),
            RankedFeature("d", -1.0),
        ]
        ordered = [f.identifier for f in sort_ranked_list(features)]
        self.assertEqual(ordered, ["c", "a", "b", "d"])

    def test_prepare_set_dedup_and_missing(self):
        prep = prepare_set(["x", "y", "x", "zzz"], ["x", "y", "z"])
        self.assertEqual(prep.members, ("x", "y"))
        self.assertEqual(prep.duplicate_members, ("x",))
        self.assertEqual(prep.missing_members, ("zzz",))
        self.assertTrue(prep.computable)

    def test_prepare_set_empty_after_filtering(self):
        prep = prepare_set(["nope"], ["x", "y"])
        self.assertFalse(prep.computable)
        self.assertEqual(prep.reason, "empty_after_filtering")

    def test_prepare_set_covers_entire_list(self):
        prep = prepare_set(["x", "y"], ["x", "y"])
        self.assertFalse(prep.computable)
        self.assertEqual(prep.reason, "covers_entire_ranked_list")


class RunningEnrichmentTests(unittest.TestCase):
    def test_hits_at_top(self):
        scores = [3.0, 2.0, 1.0, -1.0]
        walk, es, pos = running_enrichment(scores, {0, 1})
        self.assertAlmostEqual(walk[0], 0.6)
        self.assertAlmostEqual(walk[1], 1.0)
        self.assertAlmostEqual(es, 1.0)
        self.assertEqual(pos, 1)
        self.assertAlmostEqual(walk[-1], 0.0, places=9)

    def test_hits_at_bottom_negative_es(self):
        scores = [3.0, 2.0, 1.0, -1.0]
        _, es, pos = running_enrichment(scores, {2, 3})
        self.assertLess(es, 0.0)
        self.assertEqual(pos, 1)

    def test_zero_weights_share_increment(self):
        scores = [0.0, 0.0, 1.0, -1.0]
        walk, es, pos = running_enrichment(scores, {0, 1})
        self.assertAlmostEqual(walk[0], 0.5)
        self.assertAlmostEqual(walk[1], 1.0)

    def test_positive_wins_tie(self):
        scores = [1.0, -1.0, -1.0, 1.0]
        _, es, _ = running_enrichment(scores, {0, 3})
        self.assertGreater(es, 0.0)

    def test_invalid_sizes_raise(self):
        with self.assertRaises(ValueError):
            running_enrichment([1.0, 2.0], set())
        with self.assertRaises(ValueError):
            running_enrichment([1.0, 2.0], {0, 1})


class PermutationTests(unittest.TestCase):
    def test_reproducible_with_same_seed(self):
        scores = [3.0, 2.0, 1.0, -1.0, -2.0]
        p1, e1 = permutation_pvalue(scores, 2, 0.9, 200, seed=42)
        p2, e2 = permutation_pvalue(scores, 2, 0.9, 200, seed=42)
        self.assertEqual((p1, e1), (p2, e2))

    def test_pvalue_never_zero(self):
        scores = [3.0, 2.0, 1.0, -1.0, -2.0]
        p, _ = permutation_pvalue(scores, 2, 1.0, 100, seed=1)
        self.assertGreaterEqual(p, 1.0 / 101.0)

    def test_extreme_observation_small_pvalue(self):
        scores = [5.0, 4.0, 3.0, 2.0, 1.0, -1.0, -2.0, -3.0]
        p, _ = permutation_pvalue(scores, 2, 1.0, 500, seed=7)
        self.assertLess(p, 0.05)

    def test_member_order_does_not_change_result(self):
        ranked = sort_ranked_list(
            [RankedFeature(f"f{i}", 5.0 - i) for i in range(10)]
        )
        r1 = compute_enrichment("s", ["f0", "f1", "f2"], ranked, 300, 9)
        r2 = compute_enrichment("s", ["f2", "f0", "f1"], ranked, 300, 9)
        self.assertEqual(r1.enrichment_score, r2.enrichment_score)
        self.assertEqual(r1.p_value, r2.p_value)
        self.assertEqual(r1.walk, r2.walk)


class BHTests(unittest.TestCase):
    def test_bh_adjust_monotone(self):
        adjusted = bh_adjust([0.01, 0.04, 0.5])
        self.assertAlmostEqual(adjusted[0], 0.03)
        self.assertAlmostEqual(adjusted[1], 0.06)
        self.assertAlmostEqual(adjusted[2], 0.5)

    def test_bh_adjust_caps_at_one(self):
        self.assertEqual(bh_adjust([0.9, 0.95]), [0.95, 0.95])
        self.assertEqual(bh_adjust([1.0]), [1.0])


class ComputeEnrichmentTests(unittest.TestCase):
    def test_uncomputable_set_has_null_statistics(self):
        ranked = sort_ranked_list(
            [RankedFeature(f"f{i}", float(5 - i)) for i in range(6)]
        )
        result = compute_enrichment("bad", ["zzz"], ranked, 100, 1)
        self.assertFalse(result.computable)
        self.assertIsNone(result.enrichment_score)
        self.assertIsNone(result.p_value)
        self.assertIsNone(result.walk)
        self.assertEqual(result.missing_members, ("zzz",))


if __name__ == "__main__":
    unittest.main()
