import json
import tempfile
import unittest
from pathlib import Path

from ancestral_parsimony.data import load_costs, load_traits, validate_observations
from ancestral_parsimony.errors import InputError
from ancestral_parsimony.tree import load_tree


def write(tmp, name, obj):
    path = Path(tmp) / name
    if isinstance(obj, str):
        path.write_text(obj, encoding="utf-8")
    else:
        path.write_text(json.dumps(obj), encoding="utf-8")
    return str(path)


class TreeValidationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = self.tmp.name

    def load(self, obj):
        return load_tree(write(self.dir, "tree.json", obj))

    def test_valid_multifurcation(self):
        tree = self.load({"nodes": [
            {"id": 1, "children": [2, 3, 4]},
            {"id": 2}, {"id": 3}, {"id": 4},
        ]})
        self.assertEqual(tree.root, 1)
        self.assertEqual(tree.leaves, [2, 3, 4])

    def test_cycle_rejected(self):
        with self.assertRaises(InputError):
            self.load({"nodes": [
                {"id": 1, "children": [2]},
                {"id": 2, "children": [1]},
            ]})

    def test_multiple_parents_rejected(self):
        with self.assertRaises(InputError):
            self.load({"nodes": [
                {"id": 1, "children": [3]},
                {"id": 2, "children": [3]},
                {"id": 3},
            ]})

    def test_disconnected_rejected(self):
        with self.assertRaises(InputError):
            self.load({"nodes": [
                {"id": 1, "children": [2]},
                {"id": 2},
                {"id": 9},
            ]})

    def test_duplicate_id_rejected(self):
        with self.assertRaises(InputError):
            self.load({"nodes": [{"id": 1}, {"id": 1}]})


class TraitValidationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = self.tmp.name

    def load(self, csv_text, leaves):
        return load_traits(write(self.dir, "t.csv", csv_text), leaves)

    def test_missing_row_is_error_not_unknown(self):
        with self.assertRaises(InputError):
            self.load("id,x\n2,a\n", [2, 3])

    def test_extra_row_rejected(self):
        with self.assertRaises(InputError):
            self.load("id,x\n2,a\n3,b\n4,a\n", [2, 3])

    def test_unknown_marker_ok(self):
        cols, traits = self.load("id,x,y\n2,a,?\n3,?,b\n", [2, 3])
        self.assertEqual(cols, ["x", "y"])
        self.assertEqual(traits[2], ["a", "?"])

    def test_illegal_state_rejected(self):
        _, traits = self.load("id,x\n2,z\n", [2])
        with self.assertRaises(InputError):
            validate_observations(traits, ["x"], ["a", "b"])


class CostValidationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = self.tmp.name

    def load(self, obj):
        return load_costs(write(self.dir, "c.json", obj))

    def test_valid_asymmetric(self):
        states, matrix = self.load({
            "states": ["a", "b"],
            "matrix": [[0, 5], [1, 0]],
        })
        self.assertEqual(states, ["a", "b"])
        self.assertEqual(matrix[0][1], 5)

    def test_too_many_states(self):
        with self.assertRaises(InputError):
            self.load({"states": ["1", "2", "3", "4", "5", "6", "7"],
                       "matrix": [[0] * 7] * 7})

    def test_negative_cost_rejected(self):
        with self.assertRaises(InputError):
            self.load({"states": ["a", "b"], "matrix": [[0, -1], [1, 0]]})

    def test_nonzero_diagonal_rejected(self):
        with self.assertRaises(InputError):
            self.load({"states": ["a", "b"], "matrix": [[1, 1], [1, 0]]})

    def test_incomplete_matrix_rejected(self):
        with self.assertRaises(InputError):
            self.load({"states": ["a", "b"], "matrix": [[0, 1]]})


if __name__ == "__main__":
    unittest.main()
