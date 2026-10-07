import math
import unittest

from ranked_set_enrichment.core import (
    FeatureScore,
    compute_running_sum,
    enrichment_score,
)


def ranked(scores):
    return [FeatureScore(f"f{i}", s) for i, s in enumerate(scores)]


class RunningSumTest(unittest.TestCase):
    def test_walk_ends_at_zero(self):
        walk = compute_running_sum(ranked([3.0, 2.0, 1.0, 0.5]), {0, 2})
        self.assertAlmostEqual(walk[-1], 0.0)

    def test_hit_weights_normalized_by_abs_score(self):
        walk = compute_running_sum(ranked([4.0, -1.0, 2.0, 0.5]), {0, 2})
        # hits at 0 (weight 4) and 2 (weight 2): increments 4/6, 2/6
        self.assertAlmostEqual(walk[0], 4 / 6)
        self.assertAlmostEqual(walk[2], 4 / 6 - 1 / 2 + 2 / 6)

    def test_zero_weights_fall_back_to_equal_shares(self):
        walk = compute_running_sum(ranked([0.0, 1.0, 0.0, 2.0]), {0, 2})
        self.assertAlmostEqual(walk[0], 0.5)
        self.assertAlmostEqual(walk[2], 1.0 - 0.5)


class EnrichmentScoreTest(unittest.TestCase):
    def test_picks_max_absolute_value(self):
        score, pos = enrichment_score([0.1, -0.5, 0.3])
        self.assertEqual((score, pos), (-0.5, 1))

    def test_tie_prefers_positive(self):
        score, pos = enrichment_score([-0.4, 0.4])
        self.assertEqual((score, pos), (0.4, 1))

    def test_tie_prefers_earliest_position(self):
        score, pos = enrichment_score([0.4, 0.2, 0.4])
        self.assertEqual((score, pos), (0.4, 0))


if __name__ == "__main__":
    unittest.main()
