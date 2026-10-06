import csv
import json
import os
import subprocess
import sys
import tempfile
import unittest


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            csv_path = os.path.join(tmp, "m.csv")
            cfg_path = os.path.join(tmp, "cfg.json")
            out_dir = os.path.join(tmp, "out")
            with open(csv_path, "w", newline="", encoding="utf-8") as fh:
                writer = csv.writer(fh)
                writer.writerow(["part", "operator", "replicate", "value"])
                for p, base in (("P1", 10.0), ("P2", 12.0)):
                    for op in ("A", "B"):
                        for rep in (1, 2):
                            writer.writerow(
                                [p, op, rep, base + (0.1 if rep == 1 else -0.1)]
                            )
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump({}, fh)
            result = subprocess.run(
                [sys.executable, "-m", "gage_study", csv_path, cfg_path,
                 "--out-dir", out_dir],
                capture_output=True,
                text=True,
                cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            with open(os.path.join(out_dir, "anova.csv"), encoding="utf-8") as fh:
                rows = list(csv.reader(fh))
            self.assertEqual(rows[0], ["source", "df", "ss", "ms"])
            self.assertEqual(rows[-1][0], "total")
            with open(
                os.path.join(out_dir, "variance_components.csv"), encoding="utf-8"
            ) as fh:
                vc_rows = list(csv.reader(fh))
            names = [r[0] for r in vc_rows[1:]]
            self.assertIn("gage", names)
            self.assertIn("total", names)
            with open(os.path.join(out_dir, "means.json"), encoding="utf-8") as fh:
                means = json.load(fh)
            self.assertEqual(len(means["cells"]), 4)
            with open(
                os.path.join(out_dir, "diagnostics.json"), encoding="utf-8"
            ) as fh:
                diag = json.load(fh)
            self.assertEqual(diag["n_observations"], 8)

    def test_invalid_input_exit_code(self):
        with tempfile.TemporaryDirectory() as tmp:
            csv_path = os.path.join(tmp, "m.csv")
            cfg_path = os.path.join(tmp, "cfg.json")
            with open(csv_path, "w", newline="", encoding="utf-8") as fh:
                writer = csv.writer(fh)
                writer.writerow(["part", "operator", "replicate", "value"])
                writer.writerow(["P1", "A", 1, 1.0])
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump({}, fh)
            result = subprocess.run(
                [sys.executable, "-m", "gage_study", csv_path, cfg_path],
                capture_output=True,
                text=True,
                cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            )
            self.assertEqual(result.returncode, 2)
            self.assertIn("error:", result.stderr)
