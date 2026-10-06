import unittest

from reaction_balance.balance import BalanceError, balance_reaction


class BalanceTests(unittest.TestCase):
    def test_simple(self):
        self.assertEqual(
            balance_reaction(["H2", "O2"], ["H2O"]),
            {"H2": 2, "O2": 1, "H2O": 2},
        )

    def test_parentheses_species(self):
        self.assertEqual(
            balance_reaction(["Ca(OH)2", "H3PO4"], ["Ca3(PO4)2", "H2O"]),
            {"Ca(OH)2": 3, "H3PO4": 2, "Ca3(PO4)2": 1, "H2O": 6},
        )

    def test_reduced_to_coprime(self):
        result = balance_reaction(["C2H6", "O2"], ["CO2", "H2O"])
        self.assertEqual(result, {"C2H6": 2, "O2": 7, "CO2": 4, "H2O": 6})

    def test_not_unique(self):
        # Two independent solutions: CO+O2->CO2 and H2+O2->H2O combined.
        with self.assertRaisesRegex(BalanceError, "not unique"):
            balance_reaction(["CO", "H2", "O2"], ["CO2", "H2O"])

    def test_no_solution(self):
        with self.assertRaisesRegex(BalanceError, "no solution"):
            balance_reaction(["H2"], ["H2O"])

    def test_duplicate_species_rejected(self):
        with self.assertRaisesRegex(BalanceError, "duplicate"):
            balance_reaction(["H2", "O2"], ["H2O", "H2"])

    def test_unknown_element_rejected(self):
        with self.assertRaises(Exception):
            balance_reaction(["Zz2"], ["Zz"])


if __name__ == "__main__":
    unittest.main()
