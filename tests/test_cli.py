import csv
import json
import os
import tempfile
import unittest

from settling.cli import main

CONFIG = {
    "column": {"depth": 2.0, "area": 0.5},
    "fluid": {"density": 1000.0, "viscosity": 1.0e-3},
    "gravity": 9.81,
    "classes": [
        {"name": "coarse", "diameter": 2e-4, "density": 2650.0, "mass": 2.0},
        {"name": "fine", "diameter": 5e-6, "density": 2650.0, "mass": 1.0},
    ],
}


class CliTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.config_path = os.path.join(self.tmp.name, "config.json")
        with open(self.config_path, "w", encoding="utf-8") as handle:
            json.dump(CONFIG, handle)
        self.samples_path = os.path.join(self.tmp.name, "samples.csv")
        with open(self.samples_path, "w", encoding="utf-8", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(["time", "z_top", "z_bottom"])
            writer.writerow([100.0, 0.0, 0.5])
            writer.writerow([100.0, 1.5, 2.0])
        self.outdir = os.path.join(self.tmp.name, "out")

    def test_end_to_end(self):
        code = main([self.config_path, "--samples", self.samples_path,
                     "--outdir", self.outdir])
        self.assertEqual(code, 0)
        for name in ("velocities.csv", "concentrations.csv",
                     "mass_balance.csv"):
            self.assertTrue(os.path.exists(os.path.join(self.outdir, name)))
        with open(os.path.join(self.outdir, "mass_balance.csv"),
                  encoding="utf-8") as fh:
            rows = list(csv.DictReader(fh))
        totals = [r for r in rows if r["class"] == "TOTAL"]
        for row in totals:
            closure = float(row["suspended_kg"]) + float(row["deposited_kg"])
            self.assertAlmostEqual(closure, 3.0, places=9)

    def test_invalid_config_returns_error(self):
        with open(self.config_path, "w", encoding="utf-8") as handle:
            json.dump({"column": {"depth": -1.0}}, handle)
        self.assertEqual(main([self.config_path]), 1)

    def test_sample_outside_column_returns_error(self):
        with open(self.samples_path, "a", encoding="utf-8", newline="") as fh:
            csv.writer(fh).writerow([10.0, 1.0, 5.0])
        code = main([self.config_path, "--samples", self.samples_path,
                     "--outdir", self.outdir])
        self.assertEqual(code, 1)


if __name__ == "__main__":
    unittest.main()
