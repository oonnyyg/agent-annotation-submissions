import csv
import json
import math
import tempfile
import unittest
from pathlib import Path

from circular_mixture_fit.cli import main


class CliTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = Path(self.tmp.name)

    def tearDown(self):
        self.tmp.cleanup()

    def _write(self, name, text):
        path = self.dir / name
        path.write_text(text, encoding="utf-8")
        return path

    def _run(self, data_csv, config):
        data = self._write("data.csv", data_csv)
        cfg = self._write("config.json", json.dumps(config))
        assign = self.dir / "assign.csv"
        report = self.dir / "report.json"
        code = main(
            [
                "--data", str(data),
                "--config", str(cfg),
                "--assignments", str(assign),
                "--report", str(report),
            ]
        )
        return code, assign, report

    def test_end_to_end_degrees_wraparound(self):
        rows = ["angle,weight"]
        for d in (358.0, 359.0, 1.0, 2.0, 359.5, 0.5):
            rows.append(f"{d},1.0")
        config = {
            "unit": "deg",
            "components": [{"weight": 1.0, "mu": 5.0, "kappa": 3.0}],
            "tolerance": 1e-10,
            "max_iterations": 500,
        }
        code, assign, report = self._run("\n".join(rows) + "\n", config)
        self.assertEqual(code, 0)
        rep = json.loads(report.read_text(encoding="utf-8"))
        self.assertTrue(rep["converged"])
        mu = rep["components"][0]["mu"]
        self.assertTrue(mu > 350.0 or mu < 10.0)
        with open(assign, newline="", encoding="utf-8") as fh:
            rows_out = list(csv.DictReader(fh))
        self.assertEqual(len(rows_out), 6)
        for row in rows_out:
            self.assertAlmostEqual(float(row["component_1_probability"]), 1.0, places=6)

    def test_components_sorted_by_direction(self):
        rows = ["angle,weight"]
        for d in (10, 12, 11, 200, 202, 198):
            rows.append(f"{d},1.0")
        config = {
            "unit": "deg",
            "components": [
                {"weight": 0.5, "mu": 190.0, "kappa": 3.0},
                {"weight": 0.5, "mu": 20.0, "kappa": 3.0},
            ],
        }
        code, _, report = self._run("\n".join(rows) + "\n", config)
        self.assertEqual(code, 0)
        rep = json.loads(report.read_text(encoding="utf-8"))
        mus = [c["mu"] for c in rep["components"]]
        self.assertEqual(mus, sorted(mus))

    def test_radians_unit(self):
        rows = ["angle,weight", "0.1,1.0", "6.2,1.0", "0.05,2.0"]
        config = {
            "unit": "rad",
            "components": [{"weight": 1.0, "mu": 0.0, "kappa": 5.0}],
        }
        code, _, report = self._run("\n".join(rows) + "\n", config)
        self.assertEqual(code, 0)
        rep = json.loads(report.read_text(encoding="utf-8"))
        mu = rep["components"][0]["mu"]
        self.assertTrue(mu < 0.2 or mu > 2.0 * math.pi - 0.2)

    def test_rejects_bad_weight(self):
        code, _, _ = self._run(
            "angle,weight\n10,-1.0\n",
            {"unit": "deg", "components": [{"weight": 1, "mu": 0, "kappa": 1}]},
        )
        self.assertEqual(code, 1)

    def test_rejects_nonfinite_angle(self):
        code, _, _ = self._run(
            "angle,weight\nnan,1.0\n",
            {"unit": "deg", "components": [{"weight": 1, "mu": 0, "kappa": 1}]},
        )
        self.assertEqual(code, 1)

    def test_rejects_too_many_components(self):
        config = {
            "unit": "deg",
            "components": [{"weight": 1, "mu": 0, "kappa": 1}] * 4,
        }
        code, _, _ = self._run("angle,weight\n10,1\n", config)
        self.assertEqual(code, 1)

    def test_rejects_kappa_out_of_range(self):
        config = {
            "unit": "deg",
            "components": [{"weight": 1, "mu": 0, "kappa": 99.0}],
        }
        code, _, _ = self._run("angle,weight\n10,1\n", config)
        self.assertEqual(code, 1)

    def test_not_converged_flagged(self):
        rows = ["angle,weight"] + [f"{d},1.0" for d in (0, 120, 240)]
        config = {
            "unit": "deg",
            "components": [
                {"weight": 0.5, "mu": 0.0, "kappa": 2.0},
                {"weight": 0.5, "mu": 180.0, "kappa": 2.0},
            ],
            "tolerance": 1e-15,
            "max_iterations": 2,
        }
        code, _, report = self._run("\n".join(rows) + "\n", config)
        self.assertEqual(code, 0)
        rep = json.loads(report.read_text(encoding="utf-8"))
        self.assertFalse(rep["converged"])
        self.assertIn("not converged", rep["status"])


if __name__ == "__main__":
    unittest.main()
