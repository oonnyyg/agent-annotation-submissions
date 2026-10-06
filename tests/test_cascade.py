import unittest

from flash_balance.cascade import run_two_stage
from flash_balance.flash import FlashError

FEEDS = [{"a": 30.0, "b": 20.0}, {"a": 10.0, "c": 40.0}]
K1 = {"a": 3.0, "b": 1.0, "c": 0.3}
K2 = {"a": 4.0, "b": 1.5, "c": 0.5}


class CascadeTests(unittest.TestCase):
    def test_feed_mixing(self):
        result = run_two_stage(FEEDS, K1, K2)
        self.assertEqual(result.combined_feed, {"a": 40.0, "b": 20.0, "c": 40.0})
        self.assertEqual(result.components, ["a", "b", "c"])

    def test_overall_balance_closes(self):
        result = run_two_stage(FEEDS, K1, K2)
        for bal in result.balance.values():
            self.assertAlmostEqual(bal["feed"], bal["product"], places=9)
        self.assertLess(result.max_balance_residual, 1e-9)

    def test_stage2_uses_stage1_vapor_only(self):
        result = run_two_stage(FEEDS, K1, K2)
        stage1_vapor = result.stage1.flash.vapor_flows
        self.assertEqual(result.stage2.feed, stage1_vapor)
        for comp in result.components:
            stage2_out = (result.products["stage2_liquid"][comp]
                          + result.products["stage2_vapor"][comp])
            self.assertAlmostEqual(stage2_out, stage1_vapor[comp], places=9)

    def test_no_vapor_skips_stage2(self):
        result = run_two_stage([{"a": 10.0}], {"a": 0.1}, {"a": 5.0})
        self.assertTrue(result.stage2.skipped)
        self.assertIsNone(result.stage2.flash)
        self.assertEqual(sum(result.products["stage2_liquid"].values()), 0.0)
        self.assertEqual(sum(result.products["stage2_vapor"].values()), 0.0)
        self.assertEqual(result.products["stage1_liquid"]["a"], 10.0)

    def test_all_zero_feeds_rejected(self):
        with self.assertRaises(FlashError):
            run_two_stage([{"a": 0.0}, {"b": 0.0}], {"a": 1.0, "b": 1.0},
                          {"a": 1.0, "b": 1.0})

    def test_negative_feed_rejected(self):
        with self.assertRaises(FlashError):
            run_two_stage([{"a": -5.0}], {"a": 1.0}, {"a": 1.0})


if __name__ == "__main__":
    unittest.main()
