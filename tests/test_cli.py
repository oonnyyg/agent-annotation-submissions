import csv
import json
import os
import tempfile
import unittest

from gapped_allan.cli import run


class CliCase(unittest.TestCase):
    def setUp(self):
        self.dir = tempfile.TemporaryDirectory()
        self.addCleanup(self.dir.cleanup)
        d = self.dir.name
        self.data = os.path.join(d, "data.csv")
        self.config = os.path.join(d, "config.json")
        self.out_csv = os.path.join(d, "out.csv")
        self.out_json = os.path.join(d, "out.json")

    def write_data(self, rows):
        with open(self.data, "w", newline="", encoding="utf-8") as fh:
            w = csv.writer(fh)
            w.writerow(["time", "frac_freq"])
            w.writerows(rows)

    def write_config(self, cfg):
        with open(self.config, "w", encoding="utf-8") as fh:
            json.dump(cfg, fh)

    def test_end_to_end_with_gap(self):
        rows = [(float(i), 1e-9) for i in range(50)]
        rows += [(float(100 + i), 1e-9) for i in range(50)]
        self.write_data(rows)
        self.write_config({"tau0": 1.0, "m_values": [1, 2, 4]})
        self.assertEqual(
            run(["--data", self.data, "--config", self.config,
                 "--out-csv", self.out_csv, "--out-json", self.out_json]),
            0,
        )
        with open(self.out_json, encoding="utf-8") as fh:
            report = json.load(fh)
        self.assertEqual(len(report["segments"]), 2)
        self.assertFalse(report["detrended"])
        for r in report["raw"]:
            self.assertEqual(r["status"], "ok")
            self.assertAlmostEqual(r["allan_dev"], 0.0, places=15)
        with open(self.out_csv, encoding="utf-8") as fh:
            lines = fh.read().strip().splitlines()
        self.assertEqual(len(lines), 4)  # header + 3 taus

    def test_detrend_reports_fit(self):
        rows = [(float(i), 2.0 * i + 1.0) for i in range(40)]
        self.write_data(rows)
        self.write_config({"tau0": 1.0, "m_values": [1, 2], "detrend": True})
        self.assertEqual(
            run(["--data", self.data, "--config", self.config,
                 "--out-csv", self.out_csv, "--out-json", self.out_json]),
            0,
        )
        with open(self.out_json, encoding="utf-8") as fh:
            report = json.load(fh)
        self.assertTrue(report["detrended"])
        self.assertAlmostEqual(report["trend_fit"]["slope_per_s"], 2.0, places=10)
        for r in report["detrended_results"]:
            self.assertAlmostEqual(r["allan_dev"], 0.0, places=10)
        # raw results keep the drift: avar(1) = (2*1)^2/2 = 2
        self.assertAlmostEqual(report["raw"][0]["allan_var"], 2.0, places=10)

    def test_invalid_config_does_not_overwrite(self):
        self.write_data([(0.0, 0.0), (1.0, 0.0)])
        self.write_config({"tau0": -1.0, "m_values": [1]})
        with open(self.out_csv, "w", encoding="utf-8") as fh:
            fh.write("sentinel")
        rc = run(["--data", self.data, "--config", self.config,
                  "--out-csv", self.out_csv, "--out-json", self.out_json])
        self.assertEqual(rc, 1)
        with open(self.out_csv, encoding="utf-8") as fh:
            self.assertEqual(fh.read(), "sentinel")
        self.assertFalse(os.path.exists(self.out_json))

    def test_off_grid_data_does_not_overwrite(self):
        self.write_data([(0.0, 0.0), (1.0, 0.0), (2.5, 0.0)])
        self.write_config({"tau0": 1.0, "m_values": [1]})
        with open(self.out_json, "w", encoding="utf-8") as fh:
            fh.write("sentinel")
        rc = run(["--data", self.data, "--config", self.config,
                  "--out-csv", self.out_csv, "--out-json", self.out_json])
        self.assertEqual(rc, 1)
        with open(self.out_json, encoding="utf-8") as fh:
            self.assertEqual(fh.read(), "sentinel")

    def test_insufficient_data_reported_not_zero(self):
        self.write_data([(float(i), 0.0) for i in range(5)])
        self.write_config({"tau0": 1.0, "m_values": [1, 10]})
        self.assertEqual(
            run(["--data", self.data, "--config", self.config,
                 "--out-csv", self.out_csv, "--out-json", self.out_json]),
            0,
        )
        with open(self.out_json, encoding="utf-8") as fh:
            report = json.load(fh)
        self.assertEqual(report["raw"][0]["status"], "ok")
        big = report["raw"][1]
        self.assertEqual(big["status"], "insufficient_data")
        self.assertIsNone(big["allan_dev"])


if __name__ == "__main__":
    unittest.main()
