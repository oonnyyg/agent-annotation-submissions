import unittest

from settling.config import ColumnConfig
from settling.config import ParticleClass
from settling.transport import deposited_mass
from settling.transport import interval_concentration
from settling.transport import mass_balance
from settling.transport import suspended_mass

COLUMN = ColumnConfig(depth=2.0, area=0.5)


class TransportTest(unittest.TestCase):
    def test_mass_conservation_over_time(self):
        for time in (0.0, 0.5, 2.0, 100.0):
            suspended = suspended_mass(3.0, 0.1, time, COLUMN.depth)
            deposited = deposited_mass(3.0, 0.1, time, COLUMN.depth)
            self.assertAlmostEqual(suspended + deposited, 3.0, places=12)

    def test_fully_settled_class(self):
        self.assertEqual(suspended_mass(2.0, 0.5, 10.0, COLUMN.depth), 0.0)
        self.assertEqual(deposited_mass(2.0, 0.5, 10.0, COLUMN.depth), 2.0)

    def test_partial_clear_water_layer(self):
        conc = interval_concentration(4.0, 0.5, 2.0, COLUMN, 0.0, 1.0)
        self.assertEqual(conc, 0.0)
        conc = interval_concentration(4.0, 0.5, 2.0, COLUMN, 1.0, 2.0)
        self.assertAlmostEqual(conc, 4.0)

    def test_overlap_not_centre(self):
        conc = interval_concentration(4.0, 0.5, 2.0, COLUMN, 0.5, 1.5)
        self.assertAlmostEqual(conc, 2.0)

    def test_interval_validation(self):
        with self.assertRaises(ValueError):
            interval_concentration(1.0, 0.1, 0.0, COLUMN, 1.0, 1.0)
        with self.assertRaises(ValueError):
            interval_concentration(1.0, 0.1, 0.0, COLUMN, -0.5, 1.0)
        with self.assertRaises(ValueError):
            interval_concentration(1.0, 0.1, 0.0, COLUMN, 1.0, 3.0)

    def test_negative_time_rejected(self):
        with self.assertRaises(ValueError):
            suspended_mass(1.0, 0.1, -1.0, COLUMN.depth)


class MassBalanceTest(unittest.TestCase):
    def test_totals_close(self):
        classes = [
            ParticleClass("coarse", 1e-3, 2650.0, 2.0),
            ParticleClass("fine", 1e-5, 2650.0, 1.0),
        ]
        rows, totals = mass_balance(classes, [0.2, 1e-5], 5.0, COLUMN.depth)
        self.assertEqual(len(rows), 2)
        self.assertAlmostEqual(totals["closure_error"], 0.0, places=12)
        self.assertAlmostEqual(
            totals["suspended"] + totals["deposited"], 3.0, places=12
        )
        self.assertAlmostEqual(rows[0]["suspended"], 1.0)
        self.assertAlmostEqual(rows[0]["deposited"], 1.0)


if __name__ == "__main__":
    unittest.main()
