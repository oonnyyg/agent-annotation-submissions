import csv
import json
import tempfile
import unittest
from pathlib import Path

from seqaudit.cli import main


class CliTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)
        plan = {"p0": 0.4, "p1": 0.6, "lower": -1.0, "upper": 1.0,
                "max_observations": 10, "true_p": 0.55}
        (self.dir / "plan.json").write_text(json.dumps(plan),
                                            encoding="utf-8")
        with open(self.dir / "obs.csv", "w", newline="",
                  encoding="utf-8") as handle:
            writer = csv.writer(handle)
            writer.writerow(["sequence_id", "position", "value"])
            writer.writerows([["a", 1, 1], ["a", 2, 1], ["a", 3, 1],
                              ["b", 1, 0], ["b", 2, 1]])

    def tearDown(self):
        self.tmp.cleanup()

    def run_cli(self):
        return main(["--plan", str(self.dir / "plan.json"),
                     "--observations", str(self.dir / "obs.csv"),
                     "--out-dir", str(self.dir / "out")])

    def test_outputs_created(self):
        self.assertEqual(self.run_cli(), 0)
        out = self.dir / "out"
        self.assertTrue((out / "steps.csv").exists())
        self.assertTrue((out / "stopping_probabilities.csv").exists())
        summary = json.loads((out / "summary.json").read_text(
            encoding="utf-8"))
        self.assertEqual(len(summary["sequences"]), 2)
        self.assertEqual(summary["sequences"][0]["decision"], "alternative")
        self.assertEqual(summary["sequences"][1]["decision"], "undecided")
        self.assertEqual(summary["sequences"][1]["reason"], "data_exhausted")

    def test_invalid_observations_reject_run(self):
        with open(self.dir / "obs.csv", "w", newline="",
                  encoding="utf-8") as handle:
            writer = csv.writer(handle)
            writer.writerow(["sequence_id", "position", "value"])
            writer.writerow(["a", 1, 2])
        self.assertEqual(self.run_cli(), 2)
        self.assertFalse((self.dir / "out" / "summary.json").exists())


if __name__ == "__main__":
    unittest.main()
