import unittest

from ising.exact import exact_observables
from ising.mc import RunSpec, run_temperature
from ising.model import IsingConfig
from ising.stats import summarize_samples


def make_spec(**overrides):
    params = dict(
        equilibration_sweeps=20, sampling_sweeps=100, sample_interval=5, seed=7
    )
    params.update(overrides)
    return RunSpec(**params)


class SamplingTests(unittest.TestCase):
    def test_sample_count_and_sweeps(self):
        cfg = IsingConfig(size=4, coupling=1.0, temperature=2.0)
        samples = run_temperature(cfg, make_spec(), 0)
        self.assertEqual(len(samples), 20)
        self.assertEqual([s.sweep for s in samples], list(range(5, 101, 5)))

    def test_reproducible_with_same_seed(self):
        cfg = IsingConfig(size=4, coupling=1.0, temperature=2.0)
        a = run_temperature(cfg, make_spec(), 0)
        b = run_temperature(cfg, make_spec(), 0)
        self.assertEqual(a, b)

    def test_temperature_index_changes_stream(self):
        cfg = IsingConfig(size=4, coupling=1.0, temperature=2.0)
        a = run_temperature(cfg, make_spec(), 0)
        b = run_temperature(cfg, make_spec(), 1)
        self.assertNotEqual(a, b)

    def test_reordered_temperatures_give_same_results(self):
        from ising.experiment import run_experiment

        base = dict(
            size=4, coupling=1.0, field=0.1, initial_spins=None,
            spec=make_spec(sampling_sweeps=200),
        )
        fwd = run_experiment(dict(base, temperatures=[1.5, 2.5, 3.5]))
        rev = run_experiment(dict(base, temperatures=[3.5, 2.5, 1.5]))
        for t in (1.5, 2.5, 3.5):
            self.assertEqual(fwd[t]["samples"], rev[t]["samples"])

    def test_sampling_approaches_exact_on_small_lattice(self):
        # Statistical agreement, not an exactness requirement: errors should
        # be small relative to the observable scale for a long run.
        cfg = IsingConfig(size=3, coupling=1.0, temperature=2.5)
        spec = make_spec(
            equilibration_sweeps=2000, sampling_sweeps=200000, sample_interval=10
        )
        samples = run_temperature(cfg, spec, 0)
        summary = summarize_samples(samples, 9, 2.5)
        exact = exact_observables(cfg)
        self.assertAlmostEqual(
            summary["energy_per_spin"], exact["energy_per_spin"], delta=0.02
        )
        self.assertAlmostEqual(
            summary["abs_magnetization_per_spin"],
            exact["abs_magnetization_per_spin"],
            delta=0.02,
        )


class StatsTests(unittest.TestCase):
    def test_population_moments(self):
        from ising.mc import Sample

        samples = [
            Sample(sweep=i, energy=e, magnetization=m)
            for i, (e, m) in enumerate([(0.0, 1), (2.0, -1), (4.0, 3)])
        ]
        out = summarize_samples(samples, n_sites=4, temperature=2.0)
        # mean E = 2, population var E = (4+0+4)/3 = 8/3
        self.assertAlmostEqual(out["energy_variance"], 8.0 / 3.0)
        self.assertAlmostEqual(out["energy_per_spin"], 0.5)
        # mean M = 1, population var M = (0+4+4)/3 = 8/3
        self.assertAlmostEqual(out["magnetization_variance"], 8.0 / 3.0)
        self.assertAlmostEqual(out["heat_capacity_per_spin"], (8.0 / 3.0) / 16.0)
        self.assertAlmostEqual(out["susceptibility_per_spin"], (8.0 / 3.0) / 8.0)
        self.assertAlmostEqual(out["abs_magnetization_per_spin"], 5.0 / 12.0)


if __name__ == "__main__":
    unittest.main()
