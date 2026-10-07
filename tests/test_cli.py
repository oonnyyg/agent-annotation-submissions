import csv
import json
import tempfile
import unittest
from pathlib import Path

from ranked_set_enrichment.cli import main


class CliTest(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            scores = tmp / "scores.csv"
            with open(scores, "w", newline="") as handle:
                writer = csv.writer(handle)
                writer.writerow(["feature_id", "score"])
                for i in range(20):
                    writer.writerow([f"g{i}", 10.0 - i])
            sets = tmp / "sets.json"
            sets.write_text(json.dumps({
                "sets": [
                    {"name": "top", "members": ["g0", "g1", "g2"]},
                    {"name": "gone", "members": ["zz"]},
                ]
            }), encoding="utf-8")
            summary = tmp / "summary.csv"
            curves = tmp / "curves"
            code = main([
                "--scores", str(scores), "--sets", str(sets),
                "--permutations", "50", "--seed", "5",
                "--summary", str(summary), "--curves-dir", str(curves),
            ])
            self.assertEqual(code, 0)
            with open(summary, newline="") as handle:
                rows = list(csv.DictReader(handle))
            self.assertEqual(rows[0]["status"], "ok")
            self.assertEqual(rows[1]["status"], "not_computable")
            self.assertTrue((curves / "top.csv").exists())
            self.assertFalse((curves / "gone.csv").exists())

    def test_invalid_input_returns_error_code(self):
        code = main(["--scores", "missing.csv", "--sets", "missing.json"])
        self.assertEqual(code, 2)


if __name__ == "__main__":
    unittest.main()
