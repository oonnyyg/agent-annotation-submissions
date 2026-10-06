import unittest

from settling.config import ConfigError
from settling.config import parse_config

VALID = {
    "column": {"depth": 2.0, "area": 0.5},
    "fluid": {"density": 1000.0, "viscosity": 1.0e-3},
    "gravity": 9.81,
    "classes": [
        {"name": "sand", "diameter": 2e-4, "density": 2650.0, "mass": 1.0}
    ],
}


class ConfigTest(unittest.TestCase):
    def test_valid_config(self):
        config = parse_config(VALID)
        self.assertEqual(config.column.depth, 2.0)
        self.assertEqual(config.classes[0].name, "sand")

    def test_rejects_non_positive_and_non_finite(self):
        for bad in (0.0, -1.0, float("nan"), float("inf")):
            data = dict(VALID, gravity=bad)
            with self.assertRaises(ConfigError):
                parse_config(data)

    def test_rejects_light_particles(self):
        data = dict(
            VALID,
            classes=[{"diameter": 1e-4, "density": 900.0, "mass": 1.0}],
        )
        with self.assertRaises(ConfigError):
            parse_config(data)

    def test_rejects_missing_keys_and_empty_classes(self):
        with self.assertRaises(ConfigError):
            parse_config({"column": {"depth": 1.0, "area": 1.0}})
        with self.assertRaises(ConfigError):
            parse_config(dict(VALID, classes=[]))


if __name__ == "__main__":
    unittest.main()
