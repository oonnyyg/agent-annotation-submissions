"""Configuration validation tests."""
import unittest

from dsn.config import ConfigError
from dsn.config import load_config


def minimal():
    return {
        "dt": 0.5,
        "steps": 4,
        "neurons": [
            {
                "name": "a",
                "tau": 5.0,
                "threshold": 1.0,
                "reset": 0.0,
                "v0": 0.0,
                "refractory_steps": 0,
                "stimulus": [{"start": 0.0, "end": 2.0, "v_eq": 0.0}],
            }
        ],
        "connections": [],
    }


class ConfigValidationTests(unittest.TestCase):
    def test_valid_minimal(self):
        cfg = load_config(minimal())
        self.assertEqual(cfg.steps, 4)
        self.assertEqual(cfg.neurons[0].stimulus[0].end_step, 4)

    def test_v0_must_be_below_threshold(self):
        cfg = minimal()
        cfg["neurons"][0]["v0"] = 1.0
        with self.assertRaises(ConfigError):
            load_config(cfg)

    def test_reset_must_be_below_threshold(self):
        cfg = minimal()
        cfg["neurons"][0]["reset"] = 1.5
        with self.assertRaises(ConfigError):
            load_config(cfg)

    def test_stimulus_must_align_to_grid(self):
        cfg = minimal()
        cfg["neurons"][0]["stimulus"] = [
            {"start": 0.0, "end": 1.3, "v_eq": 0.0},
            {"start": 1.3, "end": 2.0, "v_eq": 0.0},
        ]
        with self.assertRaises(ConfigError):
            load_config(cfg)

    def test_stimulus_must_cover_full_run(self):
        cfg = minimal()
        cfg["neurons"][0]["stimulus"] = [{"start": 0.0, "end": 1.0, "v_eq": 0.0}]
        with self.assertRaises(ConfigError):
            load_config(cfg)

    def test_stimulus_gap_rejected(self):
        cfg = minimal()
        cfg["neurons"][0]["stimulus"] = [
            {"start": 0.0, "end": 0.5, "v_eq": 0.0},
            {"start": 1.0, "end": 2.0, "v_eq": 0.0},
        ]
        with self.assertRaises(ConfigError):
            load_config(cfg)

    def test_delay_must_be_at_least_one(self):
        cfg = minimal()
        cfg["connections"] = [{"from": "a", "to": "a", "weight": 0.1, "delay": 0}]
        with self.assertRaises(ConfigError):
            load_config(cfg)

    def test_unknown_connection_endpoint(self):
        cfg = minimal()
        cfg["connections"] = [{"from": "a", "to": "ghost", "weight": 0.1, "delay": 1}]
        with self.assertRaises(ConfigError):
            load_config(cfg)


if __name__ == "__main__":
    unittest.main()
