import unittest
from fractions import Fraction

from reaction_balance.engine import DefinitionError, load_definition, simulate


def make_definition(reactions, operations):
    return load_definition({"reactions": reactions, "operations": operations})


class ValidationTests(unittest.TestCase):
    def test_all_validated_before_simulation(self):
        # Bad second operation must abort everything (no partial run).
        with self.assertRaises(DefinitionError):
            make_definition(
                [{"name": "r", "reactants": ["H2", "O2"], "products": ["H2O"]}],
                [
                    {"type": "add", "species": "H2", "moles": "1"},
                    {"type": "react", "reaction": "nope", "fraction": "0.5"},
                ],
            )

    def test_negative_amount_rejected(self):
        with self.assertRaises(DefinitionError):
            make_definition(
                [{"name": "r", "reactants": ["H2", "O2"], "products": ["H2O"]}],
                [{"type": "add", "species": "H2", "moles": "-1"}],
            )

    def test_fraction_above_one_rejected(self):
        with self.assertRaises(DefinitionError):
            make_definition(
                [{"name": "r", "reactants": ["H2", "O2"], "products": ["H2O"]}],
                [{"type": "react", "reaction": "r", "fraction": "1.5"}],
            )


class SimulationTests(unittest.TestCase):
    def test_chained_reactions_and_limiting(self):
        reactions, operations = make_definition(
            [
                {"name": "synth", "reactants": ["H2", "O2"], "products": ["H2O"]},
                {"name": "electro", "reactants": ["H2O"], "products": ["H2", "O2"]},
            ],
            [
                {"type": "add", "species": "H2", "moles": "2.5"},
                {"type": "add", "species": "O2", "moles": "1"},
                {"type": "react", "reaction": "synth", "fraction": "1"},
                {"type": "react", "reaction": "electro", "fraction": "0.5"},
            ],
        )
        steps = simulate(reactions, operations)
        synth = steps[3]
        # O2 limits: extent max = 1 mol, consumes 2 H2 + 1 O2, makes 2 H2O.
        self.assertEqual(synth["limiting_reactants"], ["O2"])
        self.assertEqual(synth["stock"]["H2"], Fraction(1, 2))
        self.assertNotIn("O2", synth["stock"])
        self.assertEqual(synth["stock"]["H2O"], Fraction(2))
        electro = steps[4]
        # 0.5 * max extent (2 mol H2O) = 1 mol extent.
        self.assertEqual(electro["stock"]["H2O"], Fraction(1))
        self.assertEqual(electro["stock"]["H2"], Fraction(3, 2))
        self.assertEqual(electro["stock"]["O2"], Fraction(1, 2))

    def test_tied_limiting_reactants(self):
        reactions, operations = make_definition(
            [{"name": "r", "reactants": ["H2", "O2"], "products": ["H2O"]}],
            [
                {"type": "add", "species": "H2", "moles": "2"},
                {"type": "add", "species": "O2", "moles": "1"},
                {"type": "react", "reaction": "r", "fraction": "1"},
            ],
        )
        steps = simulate(reactions, operations)
        self.assertEqual(steps[3]["limiting_reactants"], ["H2", "O2"])
        self.assertEqual(steps[3]["stock"]["H2O"], Fraction(2))

    def test_decimal_feed_is_exact(self):
        reactions, operations = make_definition(
            [{"name": "r", "reactants": ["N2", "H2"], "products": ["NH3"]}],
            [
                {"type": "add", "species": "N2", "moles": "0.1"},
                {"type": "add", "species": "H2", "moles": "0.3"},
                {"type": "react", "reaction": "r", "fraction": "1"},
            ],
        )
        steps = simulate(reactions, operations)
        # 0.1 and 0.3 are exact decimals, not binary floats.
        self.assertEqual(steps[3]["stock"]["NH3"], Fraction(1, 5))
        self.assertEqual(steps[3]["limiting_reactants"], ["H2", "N2"])

    def test_missing_reactant_gives_zero_extent(self):
        reactions, operations = make_definition(
            [{"name": "r", "reactants": ["H2", "O2"], "products": ["H2O"]}],
            [
                {"type": "add", "species": "H2", "moles": "5"},
                {"type": "react", "reaction": "r", "fraction": "1"},
            ],
        )
        steps = simulate(reactions, operations)
        self.assertEqual(steps[2]["stock"]["H2"], Fraction(5))
        self.assertNotIn("H2O", steps[2]["stock"])


if __name__ == "__main__":
    unittest.main()
