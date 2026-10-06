import csv
import json
import tempfile
import unittest
from pathlib import Path

from rainflow_fatigue.cli import main, run


class CliTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)
        history = self.dir / "history.csv"
        history.write_text(
            "time,stress\n"
            "0.0,0.0\n1.0,10.0\n2.0,4.0\n3.0,6.0\n"
            "4.0,4.0\n5.0,10.0\n6.0,0.0\n",
            encoding="utf-8")
        config = self.dir / "config.json"
        config.write_text(json.dumps({
            "reference_stress": 200.0,
            "exponent": 3.0,
            "mean_correction": "goodman",
            "ultimate_strength": 500.0,
        }), encoding="utf-8")
        self.history = history
        self.config = config

    def test_end_to_end_outputs(self):
        cycles_csv = self.dir / "cycles.csv"
        summary_json = self.dir / "summary.json"
        code = main([str(self.history), str(self.config),
                     "--cycles-out", str(cycles_csv),
                     "--summary-out", str(summary_json)])
        self.assertEqual(code, 0)
        summary = json.loads(summary_json.read_text(encoding="utf-8"))
        self.assertEqual(summary["full_cycles"], 2)
        self.assertEqual(summary["half_cycles"], 2)
        self.assertTrue(summary["evaluable"])
        self.assertIsNotNone(summary["total_damage"])
        with open(cycles_csv, newline="", encoding="utf-8") as handle:
            rows = list(csv.DictReader(handle))
        self.assertEqual(len(rows), len(summary["cycles"]))
        self.assertIn("start_time", rows[0])
        per_cycle = sum(float(r["damage"]) for r in rows)
        self.assertAlmostEqual(per_cycle, summary["total_damage"])

    def test_not_evaluable_total_is_null(self):
        self.config.write_text(json.dumps({
            "reference_stress": 200.0,
            "exponent": 3.0,
            "mean_correction": "goodman",
            "ultimate_strength": 4.0,
        }), encoding="utf-8")
        summary = run(str(self.history), str(self.config),
                      str(self.dir / "c.csv"), str(self.dir / "s.json"))
        self.assertFalse(summary["evaluable"])
        self.assertIsNone(summary["total_damage"])

    def test_rejects_non_increasing_time(self):
        self.history.write_text("time,stress\n0.0,1.0\n0.0,2.0\n",
                                encoding="utf-8")
        code = main([str(self.history), str(self.config),
                     "--cycles-out", str(self.dir / "c.csv"),
                     "--summary-out", str(self.dir / "s.json")])
        self.assertEqual(code, 1)

    def tearDown(self):
        self.tmp.cleanup()


if __name__ == "__main__":
    unittest.main()
