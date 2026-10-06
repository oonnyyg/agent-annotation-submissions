import unittest

from spin_echo.config import load_experiment
from spin_echo.errors import ConfigError


def base():
    return {
        "spins": [{"weight": 1.0, "initial": [0.0, 1.0, 0.0]}],
        "samples": [0.0, 0.1],
    }


class ConfigTests(unittest.TestCase):
    def test_negative_weight_located(self):
        data = base()
        data["spins"][0]["weight"] = -0.5
        with self.assertRaises(ConfigError) as ctx:
            load_experiment(data)
        self.assertIn("spins[0].weight", str(ctx.exception))

    def test_zero_total_weight_rejected(self):
        data = base()
        data["spins"][0]["weight"] = 0.0
        with self.assertRaises(ConfigError) as ctx:
            load_experiment(data)
        self.assertIn("total weight", str(ctx.exception))

    def test_bad_t2_located(self):
        data = base()
        data["spins"].append({"weight": 1.0, "t2": -3.0})
        with self.assertRaises(ConfigError) as ctx:
            load_experiment(data)
        self.assertIn("spins[1].t2", str(ctx.exception))

    def test_infinite_relaxation_accepted(self):
        data = base()
        data["spins"][0]["t1"] = "inf"
        data["spins"][0]["t2"] = "Infinity"
        exp = load_experiment(data)
        self.assertEqual(exp.spins[0].t1, float("inf"))
        self.assertEqual(exp.spins[0].t2, float("inf"))

    def test_bad_pulse_time_located(self):
        data = base()
        data["pulses"] = [{"time": -0.1}]
        with self.assertRaises(ConfigError) as ctx:
            load_experiment(data)
        self.assertIn("pulses[0].time", str(ctx.exception))

    def test_bad_sample_step_located(self):
        data = base()
        data["samples"] = {"start": 0.0, "stop": 1.0, "step": 0.0}
        with self.assertRaises(ConfigError) as ctx:
            load_experiment(data)
        self.assertIn("samples.step", str(ctx.exception))

    def test_duplicate_and_unordered_samples_normalized(self):
        data = base()
        data["samples"] = [0.3, 0.1, 0.1, 0.2]
        exp = load_experiment(data)
        self.assertEqual(exp.sample_times, [0.1, 0.2, 0.3])


if __name__ == "__main__":
    unittest.main()
