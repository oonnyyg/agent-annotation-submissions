import csv
import json
import tempfile
import unittest
from pathlib import Path

from flash_balance.__main__ import main

CONFIG = {
    "feeds": [{"a": 30.0, "b": 20.0}, {"a": 10.0, "c": 40.0}],
    "cases": [
        {"name": "base", "stage1_k": {"a": 3.0, "b": 1.0, "c": 0.3},
         "stage2_k": {"a": 4.0, "b": 1.5, "c": 0.5}},
        {"name": "mild", "stage1_k": {"a": 1.5, "b": 1.0, "c": 0.7},
         "stage2_k": {"a": 2.0, "b": 1.2, "c": 0.8}},
    ],
}


class CliTests(unittest.TestCase):
    def _run(self, config):
        tmp = tempfile.TemporaryDirectory()
        self.addCleanup(tmp.cleanup)
        root = Path(tmp.name)
        cfg = root / "config.json"
        cfg.write_text(json.dumps(config), encoding="utf-8")
        outdir = root / "out"
        code = main([str(cfg), "--outdir", str(outdir)])
        return code, outdir

    def test_outputs_written(self):
        code, outdir = self._run(CONFIG)
        self.assertEqual(code, 0)
        summary = json.loads((outdir / "summary.json").read_text(encoding="utf-8"))
        self.assertEqual([c["case"] for c in summary["cases"]], ["base", "mild"])
        for case in summary["cases"]:
            self.assertEqual(case["status"], "ok")
            self.assertLess(case["max_balance_residual"], 1e-9)
        rows = list(csv.DictReader((outdir / "streams_base.csv").open(encoding="utf-8")))
        self.assertEqual([r["stream"] for r in rows],
                         ["stage1_liquid", "stage2_liquid", "stage2_vapor"])
        self.assertNotEqual(rows[0]["x_a"], "")

    def test_empty_stream_composition_blank(self):
        config = {"feeds": [{"a": 10.0}],
                  "cases": [{"name": "liq", "stage1_k": {"a": 0.1},
                             "stage2_k": {"a": 5.0}}]}
        code, outdir = self._run(config)
        self.assertEqual(code, 0)
        rows = list(csv.DictReader((outdir / "streams_liq.csv").open(encoding="utf-8")))
        self.assertEqual(rows[1]["x_a"], "")
        self.assertEqual(rows[2]["total_flow"], "0.0")

    def test_case_order_independent(self):
        reversed_config = dict(CONFIG, cases=list(reversed(CONFIG["cases"])))
        code1, out1 = self._run(CONFIG)
        code2, out2 = self._run(reversed_config)
        self.assertEqual((code1, code2), (0, 0))
        self.assertEqual((out1 / "summary.json").read_text(encoding="utf-8"),
                         (out2 / "summary.json").read_text(encoding="utf-8"))

    def test_invalid_config_writes_nothing(self):
        bad = {"feeds": [{"a": -1.0}],
               "cases": [{"name": "x", "stage1_k": {"a": 1.0}, "stage2_k": {"a": 1.0}}]}
        code, outdir = self._run(bad)
        self.assertEqual(code, 2)
        self.assertFalse(outdir.exists())


if __name__ == "__main__":
    unittest.main()
