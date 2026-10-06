import csv
import json
import math
import tempfile
import unittest
from pathlib import Path

from pseudorange_lab.cli import main

RX = (1_000_000.0, -2_000_000.0, 5_000_000.0)
CLOCK = 12_345.0
SAT_POSITIONS = [
    (20_000_000.0, 5_000_000.0, 15_000_000.0),
    (-12_000_000.0, 18_000_000.0, 14_000_000.0),
    (8_000_000.0, -20_000_000.0, 13_000_000.0),
    (-15_000_000.0, -10_000_000.0, 20_000_000.0),
    (3_000_000.0, 12_000_000.0, 22_000_000.0),
]


def write_inputs(tmp: Path) -> tuple[Path, Path]:
    obs = tmp / "obs.csv"
    with open(obs, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(["epoch", "sat_id", "x", "y", "z",
                         "pseudorange", "sigma"])
        for i, pos in enumerate(SAT_POSITIONS):
            pr = math.dist(RX, pos) + CLOCK
            writer.writerow(["E1", f"G{i + 1:02d}", *pos, pr, 1.0])
        # E2: degenerate (all satellites collinear with receiver).
        direction = (1.0, 2.0, 3.0)
        norm = math.sqrt(sum(v * v for v in direction))
        unit = tuple(v / norm for v in direction)
        for i, dist in enumerate((20e6, 22e6, 25e6, 30e6)):
            pos = tuple(RX[k] + dist * unit[k] for k in range(3))
            pr = math.dist(RX, pos) + CLOCK
            writer.writerow(["E2", f"G{i + 1:02d}", *pos, pr, 1.0])
    initials = tmp / "initials.json"
    initials.write_text(json.dumps(
        {"E1": [0, 0, 0, 0], "E2": [0, 0, 0, 0]}), encoding="utf-8")
    return obs, initials


class CliTests(unittest.TestCase):
    def test_epoch_failure_does_not_block_others(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            obs, initials = write_inputs(tmp)
            json_path = tmp / "report.json"
            csv_path = tmp / "report.csv"
            code = main([str(obs), str(initials),
                         "--json-report", str(json_path),
                         "--csv-report", str(csv_path)])
            self.assertEqual(code, 1)
            report = json.loads(json_path.read_text(encoding="utf-8"))
            epochs = {e["epoch"]: e for e in report["epochs"]}
            e1 = epochs["E1"]
            self.assertEqual(e1["status"], "ok")
            self.assertAlmostEqual(e1["solution"]["x"], RX[0], places=3)
            self.assertAlmostEqual(e1["solution"]["clock_m"], CLOCK, places=3)
            self.assertEqual(len(e1["leave_one_out"]), 5)
            self.assertTrue(all(r["ok"] for r in e1["leave_one_out"]))
            e2 = epochs["E2"]
            self.assertEqual(e2["status"], "failed")
            self.assertIn("RankDeficientError", e2["error"])
            rows = list(csv.DictReader(open(csv_path, encoding="utf-8")))
            self.assertEqual(len(rows), 6)  # 5 LOO rows for E1 + 1 for E2

    def test_missing_initial_reported(self):
        with tempfile.TemporaryDirectory() as tmpdir:
            tmp = Path(tmpdir)
            obs, _ = write_inputs(tmp)
            initials = tmp / "initials.json"
            initials.write_text(json.dumps({"E1": [0, 0, 0, 0]}),
                                encoding="utf-8")
            json_path = tmp / "report.json"
            code = main([str(obs), str(initials),
                         "--json-report", str(json_path),
                         "--csv-report", str(tmp / "report.csv")])
            self.assertEqual(code, 1)
            report = json.loads(json_path.read_text(encoding="utf-8"))
            epochs = {e["epoch"]: e for e in report["epochs"]}
            self.assertEqual(epochs["E1"]["status"], "ok")
            self.assertEqual(epochs["E2"]["status"], "failed")
            self.assertIn("MissingInitialError", epochs["E2"]["error"])


if __name__ == "__main__":
    unittest.main()
