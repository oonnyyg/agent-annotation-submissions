import unittest

from ising_lab.config import ConfigError, parse_config


def base_config(**overrides):
    data = {
        "size": 4,
        "coupling": 1.0,
        "field": 0.0,
        "seed": 1,
        "temperatures": [{"temperature": 2.0, "n_samples": 10}],
    }
    data.update(overrides)
    return data


class TestConfig(unittest.TestCase):
    def test_valid_config_defaults(self):
        config = parse_config(base_config())
        self.assertEqual(config.n_spins, 16)
        self.assertEqual(config.initial_spins, (1,) * 16)
        self.assertEqual(config.temperatures[0].thermalization_sweeps, 100)
        self.assertEqual(config.temperatures[0].sample_every, 1)

    def test_size_bounds(self):
        for bad in (2, 65, 3.5, "4"):
            with self.assertRaises(ConfigError):
                parse_config(base_config(size=bad))
        for good in (3, 64):
            parse_config(base_config(size=good))

    def test_coupling_and_temperature_must_be_positive_finite(self):
        for bad in (0, -1.0, float("nan"), float("inf")):
            with self.assertRaises(ConfigError):
                parse_config(base_config(coupling=bad))
            with self.assertRaises(ConfigError):
                parse_config(base_config(temperatures=[bad]))

    def test_field_must_be_finite(self):
        with self.assertRaises(ConfigError):
            parse_config(base_config(field=float("nan")))
        parse_config(base_config(field=-2.5))

    def test_duplicate_temperature_rejected(self):
        with self.assertRaises(ConfigError):
            parse_config(base_config(temperatures=[2.0, {"temperature": 2.0}]))

    def test_initial_spins_validation(self):
        with self.assertRaises(ConfigError):
            parse_config(base_config(initial_spins=[1] * 15))
        with self.assertRaises(ConfigError):
            parse_config(base_config(initial_spins=[1] * 15 + [0]))
        config = parse_config(base_config(initial_spins=[-1] * 16))
        self.assertEqual(config.initial_spins, (-1,) * 16)

    def test_sampling_params_must_be_positive_int(self):
        bad_temp = {"temperature": 2.0, "sample_every": 0}
        with self.assertRaises(ConfigError):
            parse_config(base_config(temperatures=[bad_temp]))
        bad_temp = {"temperature": 2.0, "n_samples": 1.5}
        with self.assertRaises(ConfigError):
            parse_config(base_config(temperatures=[bad_temp]))


if __name__ == "__main__":
    unittest.main()
