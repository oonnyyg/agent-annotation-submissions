import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from gage_study.data import (InputError, Observation, load_observations,
                             validate_structure)
from gage_study.model import analyze


def make_obs(grid):
    obs = []
    for part, operators in grid.items():
        for op, values in operators.items():
            for i, v in enumerate(values, start=1):
                obs.append(Observation(part, op, i, float(v)))
    return obs


def run_analysis(grid):
    obs = make_obs(grid)
    parts, operators, n = validate_structure(obs)
    return analyze(obs, parts, operators, n)


class PureNoiseTest(unittest.TestCase):
    def test_only_repeatability(self):
        grid = {f"P{p}": {f"O{o}": [10.0 + (r % 3 - 1) * 0.01
                                    for r in range(1, 4)]
                          for o in range(1, 3)}
                for p in range(1, 4)}
        result = run_analysis(grid)
        vc = result.variance_components
        for name in ("part", "operator", "part_operator"):
            self.assertAlmostEqual(vc[name], 0.0, delta=1e-3)
        self.assertGreater(vc["repeatability"], 0.0)
        self.assertTrue(result.ss_check["ok"])
        self.assertAlmostEqual(
            result.summary["gage"]["percent_variance"], 100.0, places=9)


class InteractionTest(unittest.TestCase):
    def test_clear_interaction(self):
        grid = {
            "P1": {"O1": [10.0, 10.1], "O2": [12.0, 12.1]},
            "P2": {"O1": [12.0, 12.1], "O2": [10.0, 10.1]},
        }
        result = run_analysis(grid)
        vc = result.variance_components
        self.assertGreater(vc["part_operator"], 1.0)
        self.assertLess(vc["repeatability"], 0.01)
        self.assertGreater(result.summary["reproducibility"]["variance"], 1.0)


class ConstantTest(unittest.TestCase):
    def test_all_constant(self):
        grid = {f"P{p}": {f"O{o}": [5.0, 5.0, 5.0] for o in (1, 2)}
                for p in (1, 2, 3)}
        result = run_analysis(grid)
        self.assertEqual(result.summary["total"]["variance"], 0.0)
        for row in result.summary.values():
            self.assertIsNone(row["percent_variance"])
            self.assertIsNone(row["percent_stddev"])


class NegativeVarianceTest(unittest.TestCase):
    def test_negative_kept_with_warning(self):
        grid = {
            "P1": {"O1": [1.0, 3.0], "O2": [1.0, 3.0]},
            "P2": {"O1": [1.0, 3.0], "O2": [1.0, 3.0]},
        }
        result = run_analysis(grid)
        self.assertLess(result.variance_components["part_operator"], 0.0)
        self.assertTrue(any("part_operator" in w for w in result.warnings))
        self.assertEqual(result.summary["part_operator"]["variance"], 0.0)


class ValidationTest(unittest.TestCase):
    def test_duplicate_replicate_rejected(self):
        obs = make_obs({"P1": {"O1": [1, 2], "O2": [1, 2]},
                        "P2": {"O1": [1, 2], "O2": [1, 2]}})
        obs.append(Observation("P1", "O1", 1, 9.0))
        with self.assertRaises(InputError):
            validate_structure(obs)

    def test_missing_cell_rejected(self):
        obs = make_obs({"P1": {"O1": [1, 2], "O2": [1, 2]},
                        "P2": {"O1": [1, 2]}})
        with self.assertRaises(InputError):
            validate_structure(obs)

    def test_unbalanced_rejected(self):
        obs = make_obs({"P1": {"O1": [1, 2, 3], "O2": [1, 2]},
                        "P2": {"O1": [1, 2], "O2": [1, 2]}})
        with self.assertRaises(InputError):
            validate_structure(obs)

    def test_order_invariance(self):
        grid = {"P1": {"O1": [1.0, 2.0], "O2": [3.0, 4.0]},
                "P2": {"O1": [5.0, 6.0], "O2": [7.0, 8.0]}}
        r1 = run_analysis(grid)
        obs = list(reversed(make_obs(grid)))
        parts, operators, n = validate_structure(obs)
        r2 = analyze(obs, parts, operators, n)
        self.assertEqual(r1.variance_components, r2.variance_components)


class CliTest(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            csv_path = os.path.join(tmp, "m.csv")
            with open(csv_path, "w", encoding="utf-8", newline="") as fh:
                fh.write("part,operator,replicate,value\n")
                for p in (1, 2, 3):
                    for o in (1, 2):
                        for r in (1, 2):
                            fh.write(f"P{p},O{o},{r},{p * 10 + r * 0.1}\n")
            cfg_path = os.path.join(tmp, "cfg.json")
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump({}, fh)
            outdir = os.path.join(tmp, "out")
            proc = subprocess.run(
                [sys.executable, "-m", "gage_study", csv_path, cfg_path,
                 "--outdir", outdir],
                capture_output=True, text=True)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            for name in ("anova.csv", "variance_components.csv",
                         "diagnostics.json"):
                self.assertTrue(os.path.exists(os.path.join(outdir, name)))
            with open(os.path.join(outdir, "diagnostics.json"),
                      encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertTrue(diag["ss_check"]["ok"])
            self.assertEqual(len(diag["cell_means"]), 6)

    def test_nonfinite_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            csv_path = os.path.join(tmp, "m.csv")
            with open(csv_path, "w", encoding="utf-8") as fh:
                fh.write("part,operator,replicate,value\nP1,O1,1,nan\n")
            cfg_path = os.path.join(tmp, "cfg.json")
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump({}, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "gage_study", csv_path, cfg_path],
                capture_output=True, text=True)
            self.assertEqual(proc.returncode, 2)
            self.assertIn("not finite", proc.stderr)


if __name__ == "__main__":
    unittest.main()
