import csv
import io
import json
import tempfile
import unittest
from pathlib import Path

from pipenet.cli import main


NETWORK = {
    "fixed_heads": {"R": 100.0},
    "pipes": [
        {"name": "p1", "from": "R", "to": "A", "resistance": 2.0},
        {"name": "p2", "from": "A", "to": "B", "resistance": 8.0},
    ],
}


def scenario_set(order):
    cases = {
        "low": {"A": 1.0, "B": 2.0},
        "high": {"A": 3.0, "B": 4.0},
    }
    return {"scenarios": [{"name": name, "demands": cases[name]} for name in order]}


class CliBatchTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)
        (self.dir / "net.json").write_text(json.dumps(NETWORK), encoding="utf-8")

    def tearDown(self):
        self.tmp.cleanup()

    def run_cli(self, order):
        tag = "-".join(order)
        scen_path = self.dir / f"scen-{tag}.json"
        scen_path.write_text(json.dumps(scenario_set(order)), encoding="utf-8")
        csv_path = self.dir / f"pipes-{tag}.csv"
        json_path = self.dir / f"nodes-{tag}.json"
        code = main([
            str(self.dir / "net.json"), str(scen_path),
            "--pipes-csv", str(csv_path),
            "--nodes-json", str(json_path),
        ])
        self.assertEqual(code, 0)
        return csv_path, json_path

    def test_order_does_not_change_results(self):
        _, json_fwd = self.run_cli(["low", "high"])
        _, json_rev = self.run_cli(["high", "low"])
        fwd = {e["scenario"]: e for e in json.loads(json_fwd.read_text())["scenarios"]}
        rev = {e["scenario"]: e for e in json.loads(json_rev.read_text())["scenarios"]}
        for name in ("low", "high"):
            self.assertEqual(fwd[name]["heads"], rev[name]["heads"])
            self.assertEqual(fwd[name]["boundary_supply"], rev[name]["boundary_supply"])

    def test_reports_balance_and_supply(self):
        csv_path, json_path = self.run_cli(["low", "high"])
        report = json.loads(json_path.read_text(encoding="utf-8"))["scenarios"]
        low = next(e for e in report if e["scenario"] == "low")
        self.assertTrue(low["converged"])
        self.assertAlmostEqual(low["boundary_supply"]["R"], 3.0, places=9)
        self.assertAlmostEqual(low["total_supply"], low["total_demand"], places=9)
        self.assertAlmostEqual(low["balance_error"], 0.0, places=9)
        rows = list(csv.DictReader(io.StringIO(csv_path.read_text(encoding="utf-8"))))
        p1 = next(r for r in rows if r["scenario"] == "low" and r["pipe"] == "p1")
        self.assertAlmostEqual(float(p1["flow"]), 3.0, places=9)

    def test_failed_scenario_not_reported_as_success(self):
        scen = {"scenarios": [{"name": "tight", "demands": {"A": 1.0}}]}
        scen_path = self.dir / "scen-tight.json"
        scen_path.write_text(json.dumps(scen), encoding="utf-8")
        csv_path = self.dir / "pipes-tight.csv"
        json_path = self.dir / "nodes-tight.json"
        code = main([
            str(self.dir / "net.json"), str(scen_path),
            "--pipes-csv", str(csv_path),
            "--nodes-json", str(json_path),
            "--max-iterations", "1",
            "--node-tolerance", "1e-15",
            "--pipe-tolerance", "1e-15",
        ])
        self.assertEqual(code, 1)
        report = json.loads(json_path.read_text(encoding="utf-8"))["scenarios"][0]
        self.assertFalse(report["converged"])
        self.assertNotIn("heads", report)
        rows = list(csv.DictReader(io.StringIO(csv_path.read_text(encoding="utf-8"))))
        self.assertEqual(rows[0]["converged"], "false")
        self.assertEqual(rows[0]["flow"], "")


if __name__ == "__main__":
    unittest.main()
