"""Data loading and validation tests."""
import json
import os
import tempfile
import unittest
from pseudorange_lab.data import load_initial_json, load_observations_csv
from pseudorange_lab.errors import DataError

CSV_GOOD = """epoch,sat_id,x_m,y_m,z_m,pseudorange_m,sigma_m
e1,G01,20000000,0,15000000,22000000,1.0
e1,G02,-18000000,8000000,12000000,23000000,1.5
e1,G03,5000000,19000000,-14000000,24000000,2.0
e1,G04,-6000000,-17000000,16000000,25000000,1.0
e2,G01,20000000,0,15000000,22000000,1.0
e2,G02,-18000000,8000000,12000000,23000000,1.5
e2,G03,5000000,19000000,-14000000,24000000,2.0
e2,G04,-6000000,-17000000,16000000,25000000,1.0
"""
INIT_GOOD = {
    "epochs": {
        "e1": {"position_m": [6378000.0, 0.0, 0.0], "clock_m": 0.0},
        "e2": {"position_m": [6378000.0, 0.0, 0.0], "clock_m": 100.0},
    }
}


def write_temp(text, suffix):
    fd, path = tempfile.mkstemp(suffix=suffix)
    with os.fdopen(fd, "w", encoding="utf-8") as handle:
        handle.write(text)
    return path


class LoadCsvTest(unittest.TestCase):
    def setUp(self):
        self.paths = []

    def tearDown(self):
        for path in self.paths:
            os.unlink(path)

    def csv(self, text):
        path = write_temp(text, ".csv")
        self.paths.append(path)
        return path

    def test_groups_epochs(self):
        epochs = load_observations_csv(self.csv(CSV_GOOD))
        self.assertEqual(sorted(epochs), ["e1", "e2"])
        self.assertEqual(len(epochs["e1"].observations), 4)
        self.assertEqual(epochs["e1"].observations[0].sat_id, "G01")

    def test_rejects_negative_sigma(self):
        bad = CSV_GOOD.replace("22000000,1.0", "22000000,-1.0", 1)
        with self.assertRaises(DataError):
            load_observations_csv(self.csv(bad))

    def test_rejects_non_positive_pseudorange(self):
        bad = CSV_GOOD.replace("22000000", "-5", 1)
        with self.assertRaises(DataError):
            load_observations_csv(self.csv(bad))

    def test_rejects_duplicate_sat_in_epoch(self):
        bad = CSV_GOOD + "e1,G01,1,2,3,20000000,1.0\n"
        with self.assertRaises(DataError):
            load_observations_csv(self.csv(bad))

    def test_rejects_fewer_than_four(self):
        bad = ("epoch,sat_id,x_m,y_m,z_m,pseudorange_m,sigma_m\n"
               "e1,G01,1,2,3,20000000,1.0\n")
        with self.assertRaises(DataError):
            load_observations_csv(self.csv(bad))

    def test_rejects_non_finite(self):
        bad = CSV_GOOD.replace("15000000", "nan", 1)
        with self.assertRaises(DataError):
            load_observations_csv(self.csv(bad))


class LoadInitialTest(unittest.TestCase):
    def test_attaches_initial_values(self):
        csv_path = write_temp(CSV_GOOD, ".csv")
        json_path = write_temp(json.dumps(INIT_GOOD), ".json")
        self.addCleanup(os.unlink, csv_path)
        self.addCleanup(os.unlink, json_path)
        epochs = load_observations_csv(csv_path)
        load_initial_json(json_path, epochs)
        self.assertEqual(epochs["e1"].initial_clock_m, 0.0)
        self.assertEqual(epochs["e2"].initial_clock_m, 100.0)
        self.assertEqual(epochs["e1"].initial_position_m[0], 6378000.0)

    def test_missing_epoch_initial_raises(self):
        csv_path = write_temp(CSV_GOOD, ".csv")
        payload = {"epochs": {"e1": INIT_GOOD["epochs"]["e1"]}}
        json_path = write_temp(json.dumps(payload), ".json")
        self.addCleanup(os.unlink, csv_path)
        self.addCleanup(os.unlink, json_path)
        epochs = load_observations_csv(csv_path)
        with self.assertRaises(DataError):
            load_initial_json(json_path, epochs)


if __name__ == "__main__":
    unittest.main()
