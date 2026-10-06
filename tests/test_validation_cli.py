import json
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from closed_network_mva.model import ConfigError, load_config
from tests.test_mva import base_config, write_config


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = ROOT / "tests" / ".tmp"
        self.tmp.mkdir(exist_ok=True)

    def assert_invalid(self, mutate, pattern):
        data = base_config()
        mutate(data)
        path = write_config(self.tmp, data)
        with self.assertRaisesRegex(ConfigError, pattern):
            load_config(path)

    def test_row_sum(self):
        self.assert_invalid(lambda d: d["routing"].__setitem__(0, [0.0, 0.5, 0.3]),
                            "sums to")

    def test_not_strongly_connected(self):
        self.assert_invalid(
            lambda d: d["routing"].__setitem__(2, [0.0, 0.0, 1.0]),
            "strongly connected")

    def test_negative_probability(self):
        self.assert_invalid(
            lambda d: d["routing"].__setitem__(0, [-0.1, 0.8, 0.3]),
            "non-negative")

    def test_bad_service_time(self):
        self.assert_invalid(
            lambda d: d["stations"][0].__setitem__("mean_service_time", 0.0),
            "positive")

    def test_unknown_reference(self):
        self.assert_invalid(lambda d: d.__setitem__("reference_station", "zz"),
                            "reference_station")

    def test_unknown_override_station(self):
        self.assert_invalid(
            lambda d: d["scenarios"][0].__setitem__(
                "service_time_overrides", {"zz": 1.0}),
            "unknown station")

    def test_bad_population(self):
        self.assert_invalid(
            lambda d: d["scenarios"][0].__setitem__("population", 0),
            "population")


class CliTests(unittest.TestCase):
    def test_cli_outputs(self):
        tmp = ROOT / "tests" / ".tmp"
        tmp.mkdir(exist_ok=True)
        csv_path = tmp / "out.csv"
        json_path = tmp / "out.json"
        proc = subprocess.run(
            [sys.executable, "-m", "closed_network_mva",
             str(ROOT / "examples" / "network.json"),
             "--csv", str(csv_path), "--json", str(json_path)],
            cwd=ROOT, capture_output=True, text=True)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        rows = csv_path.read_text(encoding="utf-8").strip().splitlines()
        self.assertEqual(len(rows), 1 + 3 * 3)
        summary = json.loads(json_path.read_text(encoding="utf-8"))
        self.assertEqual(summary["baseline_scenario"], "baseline")
        for scenario in summary["scenarios"]:
            self.assertAlmostEqual(scenario["queue_length_sum"],
                                   scenario["population"], places=6)
        faster = next(s for s in summary["scenarios"] if s["name"] == "faster-disk")
        self.assertGreater(faster["relative_throughput"], 1.0)

    def test_cli_reports_config_error(self):
        tmp = ROOT / "tests" / ".tmp"
        tmp.mkdir(exist_ok=True)
        bad = write_config(tmp, {"stations": []})
        proc = subprocess.run(
            [sys.executable, "-m", "closed_network_mva", str(bad)],
            cwd=ROOT, capture_output=True, text=True)
        self.assertEqual(proc.returncode, 2)
        self.assertIn("error:", proc.stderr)


if __name__ == "__main__":
    unittest.main()
