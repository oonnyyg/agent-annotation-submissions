import math
import unittest

from ising.exact import exact_observables
from ising.model import IsingConfig


class ExactEnumerationTests(unittest.TestCase):
    def test_rejects_large_lattice(self):
        cfg = IsingConfig(size=5, coupling=1.0, temperature=2.0)
        with self.assertRaises(ValueError):
            exact_observables(cfg)

    def test_high_temperature_limit(self):
        # At very high T all 2^N states are ~equally likely:
        # <E> -> 0 (h=0), <|M|>/N -> E|Bin(N,1/2)-N/2|*2/N, C/N -> (2J/T)^2.
        n = 9
        t = 1.0e6
        cfg = IsingConfig(size=3, coupling=1.0, temperature=t)
        exact = exact_observables(cfg)
        self.assertAlmostEqual(exact["energy_per_spin"], 0.0, delta=1e-5)
        expected_abs_m = sum(
            abs(2 * k - n) * math.comb(n, k) for k in range(n + 1)
        ) / (2**n * n)
        self.assertAlmostEqual(
            exact["abs_magnetization_per_spin"], expected_abs_m, delta=1e-5
        )
        self.assertAlmostEqual(
            exact["heat_capacity_per_spin"], (2.0 / t) ** 2, delta=1e-10
        )

    def test_low_temperature_limit(self):
        # At very low T the two ferromagnetic ground states dominate.
        cfg = IsingConfig(size=3, coupling=1.0, temperature=1.0e-3)
        exact = exact_observables(cfg)
        self.assertAlmostEqual(exact["energy_per_spin"], -2.0, places=3)
        self.assertAlmostEqual(exact["abs_magnetization_per_spin"], 1.0, places=3)
        self.assertAlmostEqual(exact["magnetization_per_spin"], 0.0, places=6)

    def test_field_breaks_magnetization_symmetry(self):
        cfg = IsingConfig(size=3, coupling=1.0, temperature=1.0, field=0.5)
        exact = exact_observables(cfg)
        self.assertGreater(exact["magnetization_per_spin"], 0.0)

    def test_state_count(self):
        cfg = IsingConfig(size=4, coupling=1.0, temperature=2.0)
        self.assertEqual(exact_observables(cfg)["n_states"], 2**16)


if __name__ == "__main__":
    unittest.main()
