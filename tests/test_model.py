import unittest

from ising.model import IsingConfig, IsingLattice


class ConfigValidationTests(unittest.TestCase):
    def test_rejects_bad_size(self):
        for bad in (2, 65, 3.5, True):
            with self.assertRaises(ValueError):
                IsingConfig(size=bad, coupling=1.0, temperature=2.0)

    def test_rejects_non_positive_coupling_or_temperature(self):
        with self.assertRaises(ValueError):
            IsingConfig(size=4, coupling=0.0, temperature=2.0)
        with self.assertRaises(ValueError):
            IsingConfig(size=4, coupling=1.0, temperature=-1.0)

    def test_rejects_non_finite_values(self):
        with self.assertRaises(ValueError):
            IsingConfig(size=4, coupling=1.0, temperature=float("inf"))
        with self.assertRaises(ValueError):
            IsingConfig(size=4, coupling=1.0, temperature=2.0, field=float("nan"))

    def test_rejects_bad_initial_spins(self):
        with self.assertRaises(ValueError):
            IsingConfig(size=3, coupling=1.0, temperature=2.0,
                        initial_spins=(1,) * 8)
        with self.assertRaises(ValueError):
            IsingConfig(size=3, coupling=1.0, temperature=2.0,
                        initial_spins=(1,) * 8 + (0,))


class EnergyTests(unittest.TestCase):
    def test_all_up_energy(self):
        cfg = IsingConfig(size=4, coupling=1.5, temperature=2.0, field=0.3)
        lat = IsingLattice(cfg)
        # 2 bonds per site, all aligned: E = -J * 2N - h * N
        self.assertAlmostEqual(lat.total_energy(), -1.5 * 2 * 16 - 0.3 * 16)

    def test_flip_delta_matches_total_energy_difference(self):
        cfg = IsingConfig(size=5, coupling=1.2, temperature=2.0, field=-0.4,
                          initial_spins=tuple(1 if i % 3 else -1 for i in range(25)))
        lat = IsingLattice(cfg)
        for index in (0, 7, 12, 24):
            before = lat.total_energy()
            delta = lat.flip_delta_energy(index)
            lat.flip(index)
            after = lat.total_energy()
            self.assertAlmostEqual(after - before, delta)

    def test_periodic_boundary_bonds(self):
        # Single down spin on an otherwise up lattice: exactly 4 broken bonds,
        # including wrap-around ones for a corner site.
        cfg = IsingConfig(size=4, coupling=1.0, temperature=2.0)
        lat = IsingLattice(cfg)
        lat.spins[0] = -1  # corner site
        self.assertAlmostEqual(lat.total_energy(), -1.0 * (2 * 16 - 8))
        self.assertEqual(lat.neighbor_sum(0), 4)


if __name__ == "__main__":
    unittest.main()
