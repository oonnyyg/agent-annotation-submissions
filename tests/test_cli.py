import csv
import json
import os
import tempfile
import unittest

from gapped_allan.cli import main


def write_csv(path, rows):
    with open(path, "w", newline="", encoding="utf-8") as fh:
        w = csv.writer(fh)
        w.writerow(["time", "value"])
        w.writerows(rows)


def write_config(path, cfg):
    with open(path, "w", encoding="utf-8") as fh:
        json.dump(cfg, fh)


class CliTests(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.dir.cleanup)
        d = self.dir.name
        self.csv_path = os.path.join(d, "data.csv")
        self.cfg_path = os.path.join(d, "cfg.json")
        self.prefix = os.path.join(d, "report")

    def run_ok(self, rows, cfg):
        write_csv(self.csv_path, rows)
        write_config(self.cfg_path, cfg)
        rc = main([self.csv_path, self.cfg_path, "--out-prefix", self.prefix])
        self.assertEqual(rc, 0)
        with open(self.prefix + ".json", encoding="utf-8") as fh:
            return json.load(fh)

    def test_end_to_end_constant(self):
        rows = [(float(i), 1e-9) for i in range(16)]
        doc = self.run_ok(rows, {"tau0": 1.0, "m": [1, 2, 4]})
        self.assertFalse(doc["detrended"])
        for s in doc["scales"]:
            self.assertEqual(s["status"], "ok")
            self.assertAlmostEqual(s["deviation"], 0.0, places=20)
        with open(self.prefix + ".csv", encoding="utf-8") as fh:
            lines = fh.read().strip().splitlines()
        self.assertEqual(len(lines), 1 + 3)

    def test_detrend_reports_fit_and_raw(self):
        rows = [(float(i), 1e-9 * i) for i in range(20)]
        doc = self.run_ok(rows, {"tau0": 1.0, "m": [1, 2], "detrend": True})
        self.assertTrue(doc["detrended"])
        self.assertAlmostEqual(doc["fit"]["slope"], 1e-9)
        self.assertIn("raw_scales", doc)
        for s in doc["scales"]:
            self.assertAlmostEqual(s["deviation"], 0.0, places=18)

    def test_invalid_grid_does_not_overwrite(self):
        rows = [(float(i), 0.0) for i in range(10)]
        self.run_ok(rows, {"tau0": 1.0, "m": [1]})
        with open(self.prefix + ".json", encoding="utf-8") as fh:
            before = fh.read()
        bad_rows = [(0.0, 0.0), (0.5, 0.0), (1.0, 0.0)]
        write_csv(self.csv_path, bad_rows)
        rc = main([self.csv_path, self.cfg_path, "--out-prefix", self.prefix])
        self.assertEqual(rc, 1)
        with open(self.prefix + ".json", encoding="utf-8") as fh:
            self.assertEqual(fh.read(), before)

    def test_invalid_config_does_not_overwrite(self):
        rows = [(float(i), 0.0) for i in range(10)]
        self.run_ok(rows, {"tau0": 1.0, "m": [1]})
        with open(self.prefix + ".csv", encoding="utf-8") as fh:
            before = fh.read()
        write_config(self.cfg_path, {"tau0": -1.0, "m": [1]})
        rc = main([self.csv_path, self.cfg_path, "--out-prefix", self.prefix])
        self.assertEqual(rc, 1)
        with open(self.prefix + ".csv", encoding="utf-8") as fh:
            self.assertEqual(fh.read(), before)

    def test_gap_segments_in_report(self):
        rows = [(float(i), 0.0) for i in range(4)]
        rows += [(10.0 + i, 0.0) for i in range(4)]
        doc = self.run_ok(rows, {"tau0": 1.0, "m": [1, 2]})
        self.assertEqual(doc["segments"], [4, 4])
        by_m = {s["m"]: s for s in doc["scales"]}
        self.assertEqual(by_m[1]["windows"], 6)
        self.assertEqual(by_m[2]["windows"], 2)


if __name__ == "__main__":
    unittest.main()
