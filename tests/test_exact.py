import itertools
import math
import unittest

from ising_lab.exact import exact_observables
from ising_lab.model import Lattice


def brute_force_reference(size, coupling, field, temperature):
    """直接枚举：未平移权重 exp(-E/T)，L=3 时数值安全。"""
    n = size * size
    beta = 1.0 / temperature
    z = mean_e = mean_e2 = mean_am = mean_m = mean_m2 = 0.0
    for spins in itertools.product((1, -1), repeat=n):
        lattice = Lattice(size, coupling, field, spins)
        energy = lattice.total_energy()
        mag = lattice.total_magnetization()
        w = math.exp(-beta * energy)
        z += w
        mean_e += w * energy
        mean_e2 += w * energy * energy
        mean_am += w * abs(mag)
        mean_m += w * mag
        mean_m2 += w * mag * mag
    mean_e /= z
    mean_e2 /= z
    mean_am /= z
    mean_m /= z
    mean_m2 /= z
    return {
        "energy_per_spin": mean_e / n,
        "abs_magnetization_per_spin": mean_am / n,
        "specific_heat_per_spin": (mean_e2 - mean_e**2) / (n * temperature**2),
        "susceptibility_per_spin": (mean_m2 - mean_m**2) / (n * temperature),
    }


class TestExact(unittest.TestCase):
    def test_matches_direct_enumeration_l3(self):
        for coupling, field, temp in ((1.0, 0.0, 2.0), (0.8, 0.3, 1.5), (1.2, -0.2, 3.0)):
            exact = exact_observables(3, coupling, field, temp)
            ref = brute_force_reference(3, coupling, field, temp)
            self.assertEqual(exact.n_states, 2**9)
            for key, expected in ref.items():
                self.assertAlmostEqual(getattr(exact, key), expected, places=12)

    def test_l4_state_count_and_limits(self):
        exact = exact_observables(4, 1.0, 0.0, 1.0)
        self.assertEqual(exact.n_states, 2**16)
        hot = exact_observables(4, 1.0, 0.0, 1.0e6)
        self.assertAlmostEqual(hot.susceptibility_per_spin, 1.0 / 1.0e6, places=9)
        self.assertAlmostEqual(hot.energy_per_spin, 0.0, delta=1e-5)

    def test_zero_field_symmetry(self):
        exact = exact_observables(3, 1.0, 0.0, 2.5)
        self.assertGreaterEqual(exact.abs_magnetization_per_spin, 0.0)
        self.assertGreater(exact.susceptibility_per_spin, 0.0)

    def test_rejects_unsupported_size(self):
        with self.assertRaises(ValueError):
            exact_observables(5, 1.0, 0.0, 1.0)


if __name__ == "__main__":
    unittest.main()
