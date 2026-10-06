import json
import subprocess
import sys
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

from closed_network_mva.model import ConfigError, load_config
from closed_network_mva.mva import run_mva, run_scenarios
from closed_network_mva.visits import compute_visit_ratios


def write_config(tmp: Path, data: dict) -> Path:
    path = tmp / "config.json"
    path.write_text(json.dumps(data), encoding="utf-8")
    return path


def base_config() -> dict:
    return {
        "reference_station": "a",
        "stations": [
            {"name": "a", "type": "single", "mean_service_time": 0.5},
            {"name": "b", "type": "single", "mean_service_time": 0.8},
            {"name": "c", "type": "infinite", "mean_service_time": 5.0},
        ],
        "routing": [
            [0.0, 0.7, 0.3],
            [1.0, 0.0, 0.0],
            [1.0, 0.0, 0.0],
        ],
        "scenarios": [{"name": "base", "population": 10}],
    }


class VisitRatioTests(unittest.TestCase):
    def test_normalized_to_reference(self):
        routing = [[0.0, 0.7, 0.3], [1.0, 0.0, 0.0], [1.0, 0.0, 0.0]]
        v = compute_visit_ratios(routing, 0)
        self.assertAlmostEqual(v[0], 1.0)
        self.assertAlmostEqual(v[1], 0.7)
        self.assertAlmostEqual(v[2], 0.3)

    def test_periodic_chain_converges(self):
        routing = [[0.0, 1.0], [1.0, 0.0]]
        v = compute_visit_ratios(routing, 0)
        self.assertAlmostEqual(v[0], 1.0)
        self.assertAlmostEqual(v[1], 1.0)

    def test_different_reference(self):
        routing = [[0.0, 1.0], [0.5, 0.5]]
        v = compute_visit_ratios(routing, 1)
        self.assertAlmostEqual(v[1], 1.0)
        self.assertAlmostEqual(v[0], 0.5)


class MvaTests(unittest.TestCase):
    def setUp(self):
        self.tmp = ROOT / "tests" / ".tmp"
        self.tmp.mkdir(exist_ok=True)

    def load(self, data):
        return load_config(write_config(self.tmp, data))

    def test_zero_population_state(self):
        config = self.load(base_config())
        result = run_mva(config, config.scenarios[0])
        self.assertEqual(result.zero_population_queues, [0.0, 0.0, 0.0])

    def test_queue_sum_equals_population(self):
        config = self.load(base_config())
        result = run_mva(config, config.scenarios[0])
        total = sum(s.mean_queue_length for s in result.stations)
        self.assertAlmostEqual(total, 10.0)

    def test_infinite_server_residence_is_constant(self):
        config = self.load(base_config())
        result = run_mva(config, config.scenarios[0])
        think = next(s for s in result.stations if s.kind == "infinite")
        self.assertEqual(think.residence_time, think.mean_service_time)

    def test_single_server_queues(self):
        data = base_config()
        data["scenarios"] = [
            {"name": "n1", "population": 1},
            {"name": "n2", "population": 2},
        ]
        config = self.load(data)
        r1, r2 = run_scenarios(config)
        a1 = next(s for s in r1.stations if s.name == "a")
        a2 = next(s for s in r2.stations if s.name == "a")
        self.assertAlmostEqual(a1.residence_time, 0.5)
        self.assertGreater(a2.residence_time, a1.residence_time)

    def test_known_two_station_value(self):
        data = {
            "reference_station": "x",
            "stations": [
                {"name": "x", "type": "single", "mean_service_time": 1.0},
                {"name": "y", "type": "single", "mean_service_time": 1.0},
            ],
            "routing": [[0.0, 1.0], [1.0, 0.0]],
            "scenarios": [{"name": "n2", "population": 2}],
        }
        config = self.load(data)
        result = run_mva(config, config.scenarios[0])
        self.assertAlmostEqual(result.throughput, 2.0 / 3.0)
        self.assertAlmostEqual(result.cycle_time, 3.0)

    def test_single_server_utilization_capped_at_one(self):
        data = base_config()
        data["stations"][0]["mean_service_time"] = 100.0
        data["scenarios"] = [{"name": "overload", "population": 50}]
        config = self.load(data)
        result = run_mva(config, config.scenarios[0])
        for station in result.stations:
            if station.kind == "single":
                self.assertLessEqual(station.load, 1.0 + 1e-9)
        bottleneck = next(s for s in result.stations if s.name == "a")
        self.assertGreater(bottleneck.load, 0.99)

    def test_infinite_server_load_not_capped(self):
        data = base_config()
        data["stations"][2]["mean_service_time"] = 1000.0
        config = self.load(data)
        result = run_mva(config, config.scenarios[0])
        think = next(s for s in result.stations if s.kind == "infinite")
        self.assertGreater(think.load, 1.0)

    def test_scenarios_are_independent(self):
        data = base_config()
        data["scenarios"] = [
            {"name": "n5", "population": 5},
            {"name": "n5-again", "population": 5},
        ]
        config = self.load(data)
        r1, r2 = run_scenarios(config)
        self.assertAlmostEqual(r1.throughput, r2.throughput)
        self.assertAlmostEqual(r1.relative_throughput, 1.0)
        self.assertAlmostEqual(r2.relative_throughput, 1.0)

    def test_service_time_override_improves_throughput(self):
        data = base_config()
        data["scenarios"] = [
            {"name": "base", "population": 10},
            {"name": "faster", "population": 10,
             "service_time_overrides": {"b": 0.4}},
        ]
        config = self.load(data)
        r1, r2 = run_scenarios(config)
        self.assertGreater(r2.throughput, r1.throughput)
        self.assertAlmostEqual(r2.relative_throughput,
                               r2.throughput / r1.throughput)
