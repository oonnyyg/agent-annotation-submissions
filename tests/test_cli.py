import csv
import json
import tempfile
import unittest
from pathlib import Path

from ising.cli import main
from ising.experiment import load_experiment


class CliTests(unittest.TestCase):
    def _write_config(self, directory, **overrides):
        config = {
            "size": 3,
            "coupling": 1.0,
            "field": 0.0,
            "temperatures": [1.5, 2.5],
            "equilibration_sweeps": 10,
            "sampling_sweeps": 50,
            "sample_interval": 5,
            "seed": 42,
        }
        config.update(overrides)
        path = Path(directory) / "config.json"
        path.write_text(json.dumps(config), encoding="utf-8")
        return path

    def test_end_to_end_outputs(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = self._write_config(tmp)
            csv_path = Path(tmp) / "samples.csv"
            json_path = Path(tmp) / "summary.json"
            rc = main([str(cfg), "--samples-csv", str(csv_path),
                       "--summary-json", str(json_path)])
            self.assertEqual(rc, 0)
            with open(csv_path, newline="", encoding="utf-8") as fh:
                rows = list(csv.DictReader(fh))
            self.assertEqual(len(rows), 2 * 10)
            self.assertEqual(
                set(rows[0]), {"temperature", "sweep", "energy", "magnetization"}
            )
            summary = json.loads(json_path.read_text(encoding="utf-8"))
            self.assertEqual(summary["lattice"]["size"], 3)
            self.assertEqual(len(summary["temperatures"]), 2)
            for row in summary["temperatures"]:
                self.assertIn("exact", row)
                self.assertIn("sampling_error", row["sampling"])

    def test_duplicate_temperatures_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = self._write_config(tmp, temperatures=[2.0, 2.0])
            with self.assertRaises(ValueError):
                load_experiment(cfg)
            rc = main([str(cfg), "--samples-csv", str(Path(tmp) / "a.csv"),
                       "--summary-json", str(Path(tmp) / "a.json")])
            self.assertEqual(rc, 2)

    def test_invalid_size_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = self._write_config(tmp, size=2)
            with self.assertRaises(ValueError):
                load_experiment(cfg)


if __name__ == "__main__":
    unittest.main()
