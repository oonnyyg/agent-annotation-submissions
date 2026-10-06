"""End-to-end CLI test."""
from __future__ import annotations

import json
import tempfile
import unittest
from pathlib import Path

from rainflow_audit.cli import main


class TestCli(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            csv_path = tmp_path / "history.csv"
            csv_path.write_text(
                "time,stress\n0,0\n1,10\n2,-5\n3,5\n4,-10\n5,0\n",
                encoding="utf-8",
            )
            model_path = tmp_path / "model.json"
            model_path.write_text(
                json.dumps({"reference_stress": 100.0, "exponent": 5.0}),
                encoding="utf-8",
            )
            cycles_out = tmp_path / "cycles.csv"
            summary_out = tmp_path / "summary.json"
            code = main([
                str(csv_path), str(model_path),
                "--cycles-out", str(cycles_out),
                "--summary-out", str(summary_out),
            ])
            self.assertEqual(code, 0)
            summary = json.loads(summary_out.read_text(encoding="utf-8"))
            self.assertAlmostEqual(summary["cycle_count"], 2.5)
            self.assertTrue(summary["damage_complete"])
            self.assertGreater(summary["total_damage"], 0.0)
            rows = cycles_out.read_text(encoding="utf-8").strip().splitlines()
            self.assertEqual(len(rows), 1 + 4)

    def test_invalid_input_returns_error_code(self):
        with tempfile.TemporaryDirectory() as tmp:
            bad = Path(tmp) / "bad.csv"
            bad.write_text("time,stress\n0,1\n0,2\n", encoding="utf-8")
            model = Path(tmp) / "model.json"
            model.write_text('{"reference_stress": 100, "exponent": 5}',
                             encoding="utf-8")
            self.assertEqual(main([str(bad), str(model)]), 2)


if __name__ == "__main__":
    unittest.main()
