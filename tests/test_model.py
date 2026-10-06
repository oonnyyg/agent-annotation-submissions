import random
import unittest

from ising_lab.model import Lattice


def random_spins(n, rng):
    return tuple(rng.choice((1, -1)) for _ in range(n))


class TestEnergy(unittest.TestCase):
    def test_all_up_energy_counts_each_bond_once(self):
        # L=3：9 个自旋，18 条键（每方向 9 条），J=1，h=0 -> E = -18
        lattice = Lattice(3, 1.0, 0.0, (1,) * 9)
        self.assertEqual(lattice.total_energy(), -18.0)
        self.assertEqual(lattice.total_magnetization(), 9)

    def test_field_term(self):
        lattice = Lattice(3, 1.0, 0.5, (1,) * 9)
        self.assertEqual(lattice.total_energy(), -18.0 - 0.5 * 9)

    def test_periodic_boundary_neighbors(self):
        lattice = Lattice(4, 1.0, 0.0, (1,) * 16)
        self.assertEqual(lattice.neighbors(0), (12, 4, 3, 1))
        self.assertEqual(lattice.neighbors(15), (11, 3, 14, 12))

    def test_periodic_boundary_energy_matches_wrapped_bonds(self):
        spins = [1] * 9
        spins[0] = -1
        lattice = Lattice(3, 2.0, 0.0, tuple(spins))
        # 14 条对齐键 * (-2) + 4 条反对齐键 * (+2) = -28 + 8 = -20
        self.assertEqual(lattice.total_energy(), -20.0)

    def test_flip_delta_energy_matches_recompute(self):
        rng = random.Random(1234)
        for size in (3, 4, 5, 8):
            for coupling, field in ((1.0, 0.0), (0.7, 0.3), (2.5, -0.4)):
                lattice = Lattice(size, coupling, field, random_spins(size * size, rng))
                for _ in range(20):
                    index = rng.randrange(size * size)
                    before = lattice.total_energy()
                    delta = lattice.flip_delta_energy(index)
                    lattice.flip(index)
                    after = lattice.total_energy()
                    self.assertAlmostEqual(after - before, delta, places=9)
                    lattice.flip(index)

    def test_invalid_spins_rejected(self):
        with self.assertRaises(ValueError):
            Lattice(3, 1.0, 0.0, (1, 0, 1, 1, 1, 1, 1, 1, 1))
        with self.assertRaises(ValueError):
            Lattice(3, 1.0, 0.0, (1,) * 8)


if __name__ == "__main__":
    unittest.main()
