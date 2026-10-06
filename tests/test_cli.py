"""End-to-end CLI tests: reports and per-epoch failure isolation."""
import json
import math
import os
import tempfile
import unittest

from pseudorange_lab.cli import main
from test_solver import SAT_POSITIONS, TRUE_CLOCK, TRUE_POS


def build_files(tmp):
    lines = ["epoch,sat_id,x_m,y_m,z_m,pseudorange_m,sigma_m"]
    for epoch, clock in (("good", TRUE_CLOCK), ("bad", TRUE_CLOCK)):
        sats = SAT_POSITIONS if epoch == "good" else SAT_POSITIONS[:4]
        for i, pos in enumerate(sats):
            rng = math.dist(pos, TRUE_POS)
            # "bad" epoch: receiver starts on a satellite -> GeometryError
            lines.append(f"{epoch},S{i},{pos[0]},{pos[1]},{pos[2]},"
                         f"{rng + clock:.3f},1.0")
    obs_path = os.path.join(tmp, "obs.csv")
    with open(obs_path, "w", encoding="utf-8") as handle:
        handle.write("\n".join(lines) + "\n")
    init = {
        "epochs": {
            "good": {"position_m": [6.4e6, 0.0, 0.0], "clock_m": 0.0},
            "bad": {"position_m": list(SAT_POSITIONS[0]), "clock_m": 0.0},
        }
    }
    init_path = os.path.join(tmp, "init.json")
    with open(init_path, "w", encoding="utf-8") as handle:
        json.dump(init, handle)
    return obs_path, init_path


class CliTest(unittest.TestCase):
    def test_reports_and_failure_isolation(self):
        with tempfile.TemporaryDirectory() as tmp:
            obs_path, init_path = build_files(tmp)
            json_out = os.path.join(tmp, "report.json")
            csv_out = os.path.join(tmp, "report.csv")
            code = main([obs_path, init_path,
                         "--json-out", json_out, "--csv-out", csv_out])
            self.assertEqual(code, 1)  # one epoch failed
            with open(json_out, encoding="utf-8") as handle:
                report = json.load(handle)
            by_name = {e["epoch"]: e for e in report["epochs"]}
            self.assertEqual(by_name["good"]["status"], "ok")
            self.assertEqual(by_name["bad"]["status"], "error")
            pos = by_name["good"]["position_m"]
            self.assertLess(math.dist(pos, TRUE_POS), 1e-3)
            self.assertAlmostEqual(by_name["good"]["clock_m"], TRUE_CLOCK, places=3)
            self.assertIn("leave_one_out", by_name["good"])
            self.assertEqual(len(by_name["good"]["covariance"]), 4)
            with open(csv_out, encoding="utf-8") as handle:
                rows = handle.read()
            self.assertIn("good", rows)
            self.assertIn("leave_one_out", rows)


if __name__ == "__main__":
    unittest.main()
