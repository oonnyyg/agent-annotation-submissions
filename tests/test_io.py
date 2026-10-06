import csv
import io
import json
import unittest

from soil_storage.io import load_config, run_config, run_csv_batch

CONFIG = {
    "layers": [
        {"thickness": 30.0, "theta_r": 0.05, "theta_s": 0.45,
         "alpha": 0.02, "n": 1.5},
        {"thickness": 50.0, "theta_r": 0.08, "theta_s": 0.50,
         "alpha": 0.05, "n": 2.0},
        {"thickness": 20.0, "theta_r": 0.10, "theta_s": 0.40,
         "alpha": 0.10, "n": 3.0},
    ],
    "water_table": 45.0,
    "samples": [10.0, 30.0, 100.0],
    "invert": {"target": 40.0, "max_depth": 200.0},
}


class TestJson(unittest.TestCase):
    def test_run_config(self):
        config = load_config(json.dumps(CONFIG))
        report = run_config(config)
        self.assertIn("storage", report)
        self.assertEqual(report["storage"]["water_table_cm"], 45.0)
        self.assertEqual(len(report["storage"]["layers"]), 3)
        total = sum(l["storage_cm"] for l in report["storage"]["layers"])
        self.assertAlmostEqual(total, report["storage"]["total_storage_cm"])
        self.assertEqual(len(report["samples"]), 3)
        self.assertEqual(report["samples"][1]["layer_index"], 1)
        self.assertIn("inversion", report)
        self.assertTrue(report["inversion"]["solved"])

    def test_nonfinite_rejected(self):
        with self.assertRaises(ValueError):
            load_config('{"layers": [{"thickness": 1e999, "theta_r": 0.1,'
                        ' "theta_s": 0.4, "alpha": 0.02, "n": 1.5}]}')

    def test_sample_out_of_range_rejected(self):
        config = load_config(json.dumps(CONFIG))
        config["samples"] = [150.0]
        with self.assertRaises(ValueError):
            run_config(config)


class TestCsv(unittest.TestCase):
    def test_batch_order_independent(self):
        config = load_config(json.dumps(CONFIG))
        csv_a = ("kind,depth_cm\nwater_table,20\nsample,30\n"
                 "water_table,60\nsample,5\n")
        csv_b = ("kind,depth_cm\nsample,5\nwater_table,60\n"
                 "sample,30\nwater_table,20\n")
        self.assertEqual(run_csv_batch(config, csv_a),
                         run_csv_batch(config, csv_b))

    def test_batch_contents(self):
        config = load_config(json.dumps(CONFIG))
        out = run_csv_batch(config,
                            "kind,depth_cm\nwater_table,20\nsample,30\n")
        rows = list(csv.reader(io.StringIO(out)))
        self.assertEqual(rows[0],
                         ["request", "water_table_cm", "depth_cm",
                          "layer_index", "value"])
        kinds = [r[0] for r in rows[1:]]
        self.assertEqual(kinds,
                         ["total", "layer", "layer", "layer", "sample"])
        sample = rows[-1]
        self.assertEqual(sample[3], "1")  # boundary takes lower layer

    def test_sample_out_of_range_rejected(self):
        config = load_config(json.dumps(CONFIG))
        with self.assertRaises(ValueError):
            run_csv_batch(config, "kind,depth_cm\nsample,101\n")

    def test_unknown_kind_rejected(self):
        config = load_config(json.dumps(CONFIG))
        with self.assertRaises(ValueError):
            run_csv_batch(config, "kind,depth_cm\nfoo,10\n")


if __name__ == "__main__":
    unittest.main()
