import json
import os
import tempfile
import unittest

from enclosure_radiation.model import ConfigError, load_config


def base_config():
    return {
        "surfaces": [
            {"name": "a", "area": 1.0, "emissivity": 0.8},
            {"name": "b", "area": 2.0, "emissivity": 0.5},
        ],
        "view_factors": [[0.0, 1.0], [0.5, 0.5]],
        "cases": [
            {"name": "c1",
             "surfaces": [{"temperature_K": 600}, {"adiabatic": True}]},
        ],
    }


def load_dict(cfg):
    fd, path = tempfile.mkstemp(suffix=".json")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            json.dump(cfg, fh)
        return load_config(path)
    finally:
        os.unlink(path)


class ValidationTests(unittest.TestCase):
    def test_valid_config_loads(self):
        enc = load_dict(base_config())
        self.assertEqual(len(enc.surfaces), 2)
        self.assertEqual(len(enc.cases), 1)

    def test_row_sum_rejected(self):
        cfg = base_config()
        cfg["view_factors"] = [[0.0, 0.9], [0.5, 0.5]]
        with self.assertRaises(ConfigError):
            load_dict(cfg)

    def test_reciprocity_rejected(self):
        cfg = base_config()
        cfg["view_factors"] = [[0.0, 1.0], [0.6, 0.4]]
        with self.assertRaises(ConfigError):
            load_dict(cfg)

    def test_negative_view_factor_rejected(self):
        cfg = base_config()
        cfg["view_factors"] = [[-0.1, 1.1], [0.5, 0.5]]
        with self.assertRaises(ConfigError):
            load_dict(cfg)

    def test_bad_emissivity_rejected(self):
        for eps in (0.0, 1.2, -0.5):
            cfg = base_config()
            cfg["surfaces"][0]["emissivity"] = eps
            with self.assertRaises(ConfigError):
                load_dict(cfg)

    def test_bad_area_rejected(self):
        cfg = base_config()
        cfg["surfaces"][0]["area"] = 0.0
        with self.assertRaises(ConfigError):
            load_dict(cfg)

    def test_case_must_pick_exactly_one_mode(self):
        cfg = base_config()
        cfg["cases"][0]["surfaces"][1] = {}
        with self.assertRaises(ConfigError):
            load_dict(cfg)
        cfg["cases"][0]["surfaces"][1] = {
            "temperature_K": 300, "adiabatic": True}
        with self.assertRaises(ConfigError):
            load_dict(cfg)

    def test_self_view_factor_allowed(self):
        cfg = base_config()
        cfg["view_factors"] = [[0.3, 0.7], [0.35, 0.65]]
        enc = load_dict(cfg)
        self.assertAlmostEqual(enc.view_factors[0][0], 0.3)


if __name__ == "__main__":
    unittest.main()
