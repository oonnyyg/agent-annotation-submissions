"""Tests for the two-stage flash balance tool."""

import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from flash_balance.cascade import mix_feeds, run_cascade
from flash_balance.errors import FlashBalanceError
from flash_balance.flash import flash_feed


class FlashTests(unittest.TestCase):
    def test_two_phase_known_solution(self):
        r = flash_feed({"A": 50.0, "B": 50.0}, {"A": 2.0, "B": 0.5})
        self.assertEqual(r.phase, "two-phase")
        self.assertAlmostEqual(r.vapor_fraction, 0.5, places=10)
        self.assertLess(r.equilibrium_residual, 1e-9)
        for c in ("A", "B"):
            self.assertAlmostEqual(r.vapor[c] + r.liquid[c], 50.0)

    def test_all_liquid_endpoint(self):
        r = flash_feed({"A": 10.0, "B": 5.0}, {"A": 0.5, "B": 0.2})
        self.assertEqual(r.phase, "liquid")
        self.assertEqual(r.vapor_fraction, 0.0)
        self.assertEqual(sum(r.vapor.values()), 0.0)

    def test_all_vapor_endpoint(self):
        r = flash_feed({"A": 10.0, "B": 5.0}, {"A": 3.0, "B": 2.0})
        self.assertEqual(r.phase, "vapor")
        self.assertEqual(sum(r.liquid.values()), 0.0)

    def test_degenerate_all_k_one(self):
        r = flash_feed({"A": 3.0, "B": 7.0}, {"A": 1.0, "B": 1.0})
        self.assertTrue(r.degenerate)
        self.assertEqual(r.phase, "liquid")
        self.assertEqual(r.vapor_fraction, 0.0)

    def test_near_one_k_converges(self):
        k = {"A": 1.01, "B": 0.9901}
        r = flash_feed({"A": 50.0, "B": 50.0}, k)
        self.assertEqual(r.phase, "two-phase")
        self.assertGreater(r.vapor_fraction, 0.0)
        self.assertLess(r.vapor_fraction, 1.0)
        self.assertLess(r.equilibrium_residual, 1e-9)

    def test_empty_feed_rejected(self):
        with self.assertRaises(FlashBalanceError):
            flash_feed({"A": 0.0}, {"A": 2.0})

    def test_missing_k_rejected(self):
        with self.assertRaises(FlashBalanceError):
            flash_feed({"A": 1.0}, {"B": 2.0})


class CascadeTests(unittest.TestCase):
    def test_negative_flow_rejected(self):
        with self.assertRaises(FlashBalanceError):
            mix_feeds([{"A": -1.0}])

    def test_zero_total_feed_rejected(self):
        with self.assertRaises(FlashBalanceError):
            mix_feeds([{"A": 0.0}, {"B": 0.0}])

    def test_overall_balance_two_phase(self):
        r = run_cascade(
            [{"A": 60.0, "B": 40.0}, {"B": 10.0, "C": 5.0}],
            [{"A": 2.0, "B": 1.0, "C": 0.3},
             {"A": 3.0, "B": 1.2, "C": 0.2}],
        )
        for residual in r.overall_balance.values():
            self.assertAlmostEqual(residual, 0.0)
        for stage_bal in r.stage_balances:
            for residual in stage_bal.values():
                self.assertAlmostEqual(residual, 0.0)

    def test_no_vapor_skips_second_stage(self):
        r = run_cascade([{"A": 10.0}], [{"A": 0.5}, {"A": 5.0}])
        self.assertIsNone(r.stages[1].result)
        self.assertEqual(r.products["V2"].total, 0.0)
        self.assertEqual(r.products["L2"].total, 0.0)
        self.assertIsNone(r.products["V2"].composition())
        self.assertAlmostEqual(r.products["L1"].total, 10.0)

    def test_cross_stage_separation(self):
        # Stage 1 sends heavy component to L1; stage 2 splits the rest.
        r = run_cascade(
            [{"A": 50.0, "B": 50.0}],
            [{"A": 2.0, "B": 0.5}, {"A": 2.0, "B": 0.5}],
        )
        l1 = r.products["L1"]
        self.assertGreater(l1.flows["B"], l1.flows["A"])
        v2 = r.products["V2"]
        self.assertGreater(v2.flows["A"], v2.flows["B"])
        total = sum(r.products[n].total for n in ("L1", "V2", "L2"))
        self.assertAlmostEqual(total, 100.0)


class CliTests(unittest.TestCase):
    def _run(self, cfg: dict, *extra: str, cwd: str | None = None):
        with tempfile.TemporaryDirectory() as tmp:
            cfg_path = Path(tmp) / "cfg.json"
            cfg_path.write_text(json.dumps(cfg), encoding="utf-8")
            return tmp, subprocess.run(
                [sys.executable, "-m", "flash_balance", str(cfg_path), *extra],
                capture_output=True, text=True,
            )

    def _cfg(self, cases):
        return {
            "feeds": [{"name": "F", "flows": {"A": 50.0, "B": 50.0}}],
            "cases": cases,
        }

    def test_case_order_independence(self):
        case_a = {"name": "a", "stages": [
            {"k_values": {"A": 2.0, "B": 0.5}},
            {"k_values": {"A": 2.0, "B": 0.5}}]}
        case_b = {"name": "b", "stages": [
            {"k_values": {"A": 0.5, "B": 2.0}},
            {"k_values": {"A": 0.5, "B": 2.0}}]}
        _, p1 = self._run(self._cfg([case_a, case_b]))
        _, p2 = self._run(self._cfg([case_b, case_a]))
        self.assertEqual(p1.returncode, 0)
        self.assertEqual(p1.stdout, p2.stdout)

    def test_invalid_config_writes_nothing(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg_path = Path(tmp) / "bad.json"
            cfg_path.write_text(json.dumps({
                "feeds": [{"flows": {"A": -5.0}}],
                "cases": [{"name": "x", "stages": [
                    {"k_values": {"A": 1.0}}, {"k_values": {"A": 1.0}}]}],
            }), encoding="utf-8")
            streams = Path(tmp) / "s.csv"
            summary = Path(tmp) / "s.json"
            proc = subprocess.run(
                [sys.executable, "-m", "flash_balance", str(cfg_path),
                 "--streams", str(streams), "--summary", str(summary)],
                capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 1)
            self.assertFalse(streams.exists())
            self.assertFalse(summary.exists())

    def test_nonpositive_k_rejected(self):
        cfg = self._cfg([{"name": "x", "stages": [
            {"k_values": {"A": 0.0, "B": 1.0}},
            {"k_values": {"A": 1.0, "B": 1.0}}]}])
        _, proc = self._run(cfg)
        self.assertEqual(proc.returncode, 1)
        self.assertIn("positive", proc.stderr)


if __name__ == "__main__":
    unittest.main()
