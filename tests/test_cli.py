import csv
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


class CliTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.root = Path(self.tmp.name)
        scores = self.root / "scores.csv"
        scores.write_text(
            "id,score\n"
            + "".join(f"f{i},{10 - i}\n" for i in range(10)),
            encoding="utf-8",
        )
        sets = self.root / "sets.json"
        sets.write_text(
            json.dumps(
                {
                    "top_set": ["f0", "f1", "f2", "f0", "ghost"],
                    "bottom_set": ["f7", "f8", "f9"],
                    "empty_set": ["ghost"],
                }
            ),
            encoding="utf-8",
        )
        self.scores = scores
        self.sets = sets
        self.outdir = self.root / "out"

    def tearDown(self):
        self.tmp.cleanup()

    def run_cli(self, *extra):
        return subprocess.run(
            [
                sys.executable,
                "-m",
                "ranked_set_enrichment",
                "--scores",
                str(self.scores),
                "--sets",
                str(self.sets),
                "--outdir",
                str(self.outdir),
                "--permutations",
                "200",
                "--seed",
                "11",
                *extra,
            ],
            capture_output=True,
            text=True,
        )

    def test_end_to_end_outputs(self):
        proc = self.run_cli()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        summary = self.outdir / "summary.csv"
        self.assertTrue(summary.exists())
        rows = list(csv.DictReader(summary.open(encoding="utf-8")))
        by_name = {r["set"]: r for r in rows}
        top = by_name["top_set"]
        self.assertEqual(top["computable"], "True")
        self.assertEqual(top["effective_size"], "3")
        self.assertEqual(top["missing_members"], "ghost")
        self.assertEqual(top["duplicate_members"], "f0")
        self.assertGreater(float(top["enrichment_score"]), 0.0)
        self.assertTrue(top["q_value_bh"])
        bottom = by_name["bottom_set"]
        self.assertLess(float(bottom["enrichment_score"]), 0.0)
        empty = by_name["empty_set"]
        self.assertEqual(empty["computable"], "False")
        self.assertEqual(empty["reason"], "empty_after_filtering")
        self.assertEqual(empty["p_value"], "")
        curve = self.outdir / "curves" / "top_set.csv"
        self.assertTrue(curve.exists())
        curve_rows = list(csv.DictReader(curve.open(encoding="utf-8")))
        self.assertEqual(len(curve_rows), 10)
        self.assertFalse((self.outdir / "curves" / "empty_set.csv").exists())
        meta = json.loads((self.outdir / "summary.json").read_text("utf-8"))
        self.assertEqual(meta["parameters"]["seed"], 11)

    def test_seed_reproducibility(self):
        first = self.run_cli()
        self.assertEqual(first.returncode, 0, first.stderr)
        snapshot = (self.outdir / "summary.csv").read_text("utf-8")
        second = self.run_cli()
        self.assertEqual(second.returncode, 0, second.stderr)
        self.assertEqual(
            snapshot, (self.outdir / "summary.csv").read_text("utf-8")
        )

    def test_duplicate_ids_rejected(self):
        self.scores.write_text("id,score\na,1\na,2\n", encoding="utf-8")
        proc = self.run_cli()
        self.assertEqual(proc.returncode, 2)
        self.assertIn("duplicate", proc.stderr)

    def test_non_finite_score_rejected(self):
        self.scores.write_text(
            "id,score\na,1\nb,inf\nc,2\n", encoding="utf-8"
        )
        proc = self.run_cli()
        self.assertEqual(proc.returncode, 2)


if __name__ == "__main__":
    unittest.main()
