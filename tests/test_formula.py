import unittest

from reaction_balance.formula import FormulaError, parse_formula


class ParseFormulaTests(unittest.TestCase):
    def test_simple(self):
        self.assertEqual(parse_formula("H2O"), {"H": 2, "O": 1})

    def test_parentheses(self):
        self.assertEqual(parse_formula("Ca(OH)2"), {"Ca": 1, "O": 2, "H": 2})

    def test_nested_parentheses(self):
        self.assertEqual(
            parse_formula("K4(ON(SO3)2)2"),
            {"K": 4, "O": 14, "N": 2, "S": 4},
        )

    def test_repeated_element_accumulates(self):
        self.assertEqual(parse_formula("CH3COOH"), {"C": 2, "H": 4, "O": 2})

    def test_unknown_element_rejected(self):
        with self.assertRaises(FormulaError):
            parse_formula("Xx2O")

    def test_unclosed_parenthesis(self):
        with self.assertRaises(FormulaError):
            parse_formula("Ca(OH2")

    def test_stray_closing_parenthesis(self):
        with self.assertRaises(FormulaError):
            parse_formula("H2O)")

    def test_zero_subscript_rejected(self):
        with self.assertRaises(FormulaError):
            parse_formula("H0")

    def test_lowercase_start_rejected(self):
        with self.assertRaises(FormulaError):
            parse_formula("h2O")

    def test_empty_rejected(self):
        with self.assertRaises(FormulaError):
            parse_formula("")


if __name__ == "__main__":
    unittest.main()
