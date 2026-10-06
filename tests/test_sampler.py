import unittest

from ising_lab.config import parse_config
from ising_lab.exact import exact_observables
from ising_lab.sampler import run_temperature


def make_config(temperatures, seed=7, size=4, thermal=20, every=2, samples=50):
    return parse_config({
        "size": size,
        "coupling": 1.0,
        "field": 0.1,
        "seed": seed,
        "initial_spins": "all_up",
        "temperatures": [
            {
                "temperature": t,
                "thermalization_sweeps": thermal,
                "sample_every": every,
                "n_samples": samples,
            }
            for t in temperatures
        ],
    })


class TestSampler(unittest.TestCase):
    def test_fixed_seed_reproducible(self):
        config = make_config([2.0])
        first = run_temperature(config, config.temperatures[0])
        second = run_temperature(config, config.temperatures[0])
        self.assertEqual(first.samples, second.samples)

    def test_temperature_order_independent(self):
        config = make_config([1.5, 2.5, 3.5])
        shuffled = make_config([3.5, 1.5, 2.5])
        for temp in (1.5, 2.5, 3.5):
            cfg_a = next(t for t in config.temperatures if t.temperature == temp)
            cfg_b = next(t for t in shuffled.temperatures if t.temperature == temp)
            self.assertEqual(run_temperature(config, cfg_a).samples,
                             run_temperature(shuffled, cfg_b).samples)

    def test_thermalization_not_sampled_and_spacing(self):
        config = make_config([2.0], thermal=10, every=3, samples=5)
        result = run_temperature(config, config.temperatures[0])
        sweeps = [s.sweep for s in result.samples]
        self.assertEqual(sweeps, [13, 16, 19, 22, 25])
        self.assertEqual(len(result.samples), 5)

    def test_sampling_approaches_exact_on_small_lattice(self):
        # 统计一致性核查：大量样本时均值应接近精确值（非严格相等标准）
        config = make_config([2.5], size=3, thermal=200, every=5, samples=4000)
        result = run_temperature(config, config.temperatures[0])
        exact = exact_observables(3, 1.0, 0.1, 2.5)
        self.assertAlmostEqual(result.energy_per_spin, exact.energy_per_spin, delta=0.02)
        self.assertAlmostEqual(
            result.abs_magnetization_per_spin, exact.abs_magnetization_per_spin, delta=0.02
        )


if __name__ == "__main__":
    unittest.main()
