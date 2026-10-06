
import csv
import json
import os
import tempfile
import unittest

from aquifer_response.__main__ import run


BASE_CONFIG = {
    "aquifer": {"transmissivity": 1.0e-3, "storativity": 1.0e-4},
    "wells": [
        {"name": "W1", "x": 0.0, "y": 0.0,
         "schedule": [[0.0, 0.01], [3600.0, 0.0]]},
        {"name": "W2", "x": 40.0, "y": 0.0,
         "schedule": [[0.0, 0.005]]},
    ],
    "observation_points": [{"name": "obs", "x": 20.0, "y": 5.0}],
    "times": [600.0, 3600.0, 7200.0],
}


class CliTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.dir = self.tmp.name
        self.config_path = os.path.join(self.dir, "config.json")
        self.csv_path = os.path.join(self.dir, "out.csv")
        self.report_path = os.path.join(self.dir, "report.json")

    def tearDown(self):
        self.tmp.cleanup()

    def write_config(self, config):
        with open(self.config_path, "w", encoding="utf-8") as fh:
            json.dump(config, fh)

    def test_csv_and_report_written_with_contributions(self):
        self.write_config(BASE_CONFIG)
        self.assertEqual(run(self.config_path, self.csv_path, self.report_path), 0)
        with open(self.csv_path, newline="", encoding="utf-8") as fh:
            rows = list(csv.reader(fh))
        self.assertEqual(rows[0][:3], ["point", "time_s", "total_drawdown_m"])
        self.assertIn("drawdown_W1_m", rows[0])
        self.assertIn("drawdown_W2_m", rows[0])
        self.assertEqual(len(rows), 1 + 3)
        with open(self.report_path, encoding="utf-8") as fh:
            report = json.load(fh)
        contrib = report["contributions_m"]["obs"]
        totals = report["total_drawdown_m"]["obs"]
        for i in range(3):
            self.assertAlmostEqual(totals[i], contrib["W1"][i] + contrib["W2"][i])
        # recovery: after W1 stops at 3600 s its contribution declines
        self.assertLess(contrib["W1"][2], contrib["W1"][1])

    def test_candidate_ranking_and_tie_order(self):
        self.write_config(BASE_CONFIG)
        run(self.config_path, self.csv_path, self.report_path)
        with open(self.report_path, encoding="utf-8") as fh:
            totals = json.load(fh)["total_drawdown_m"]["obs"]
        config = dict(BASE_CONFIG)
        config["observed"] = [{
            "point": "obs",
            "times": [600.0, 3600.0, 7200.0],
            "drawdowns": totals,
            "weights": [1.0, 2.0, 1.0],
        }]
        config["candidates"] = [
            {"name": "wrong_T", "transmissivity": 2.0e-3, "storativity": 1.0e-4},
            {"name": "true", "transmissivity": 1.0e-3, "storativity": 1.0e-4},
            {"name": "tie_a", "transmissivity": 5.0e-3, "storativity": 5.0e-3},
            {"name": "tie_b", "transmissivity": 5.0e-3, "storativity": 5.0e-3},
        ]
        self.write_config(config)
        self.assertEqual(run(self.config_path, self.csv_path, self.report_path), 0)
        with open(self.report_path, encoding="utf-8") as fh:
            ranking = json.load(fh)["candidate_ranking"]
        self.assertEqual(ranking[0]["name"], "true")
        self.assertAlmostEqual(ranking[0]["weighted_squared_error"], 0.0, places=20)
        names = [r["name"] for r in ranking]
        self.assertLess(names.index("tie_a"), names.index("tie_b"))
        self.assertEqual([r["rank"] for r in ranking], [1, 2, 3, 4])
        self.assertEqual(ranking[0]["sample_count"], 3)

    def test_failure_does_not_overwrite_existing_report(self):
        self.write_config(BASE_CONFIG)
        self.assertEqual(run(self.config_path, self.csv_path, self.report_path), 0)
        with open(self.report_path, encoding="utf-8") as fh:
            good = fh.read()
        bad = dict(BASE_CONFIG)
        bad["aquifer"] = {"transmissivity": -1.0, "storativity": 1.0e-4}
        self.write_config(bad)
        self.assertEqual(run(self.config_path, self.csv_path, self.report_path), 1)
        with open(self.report_path, encoding="utf-8") as fh:
            self.assertEqual(fh.read(), good)

    def test_negative_weight_rejected(self):
        config = dict(BASE_CONFIG)
        config["observed"] = [{
            "point": "obs", "times": [600.0],
            "drawdowns": [0.1], "weights": [0.0],
        }]
        self.write_config(config)
        self.assertEqual(run(self.config_path, self.csv_path, self.report_path), 1)
        self.assertFalse(os.path.exists(self.report_path))


if __name__ == "__main__":
    unittest.main()
