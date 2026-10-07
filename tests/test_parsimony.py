import json
import tempfile
import unittest
from pathlib import Path

from ancestral_parsimony.cli import main
from ancestral_parsimony.model import InputError, load_problem
from ancestral_parsimony.sankoff import reconstruct

UNIT2 = [[0, 1], [1, 0]]


def write_inputs(tmp, tree_spec, csv_text):
    tree_path = Path(tmp) / "tree.json"
    csv_path = Path(tmp) / "data.csv"
    tree_path.write_text(json.dumps(tree_spec), encoding="utf-8")
    csv_path.write_text(csv_text, encoding="utf-8")
    return tree_path, csv_path


def spec(tree, states=("0", "1"), costs=UNIT2):
    return {"states": list(states), "costs": costs, "tree": tree}


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)

    def load(self, tree_spec, csv_text):
        tree_path, csv_path = write_inputs(self.tmp.name, tree_spec, csv_text)
        return load_problem(tree_path, csv_path)

    def test_duplicate_node_id_rejected(self):
        tree = spec({"id": "r", "children": [{"id": "a"}, {"id": "a"}]})
        with self.assertRaises(InputError):
            self.load(tree, "id,c\na,0\n")

    def test_missing_csv_row_rejected(self):
        tree = spec({"id": "r", "children": [{"id": "a"}, {"id": "b"}]})
        with self.assertRaisesRegex(InputError, "without a CSV row"):
            self.load(tree, "id,c\na,0\n")

    def test_extra_csv_row_rejected(self):
        tree = spec({"id": "r", "children": [{"id": "a"}]})
        with self.assertRaisesRegex(InputError, "without a matching leaf"):
            self.load(tree, "id,c\na,0\nb,1\n")

    def test_illegal_state_rejected(self):
        tree = spec({"id": "r", "children": [{"id": "a"}]})
        with self.assertRaisesRegex(InputError, "not in the state set"):
            self.load(tree, "id,c\na,X\n")

    def test_incomplete_matrix_rejected(self):
        tree = spec({"id": "r", "children": [{"id": "a"}]}, costs=[[0, 1], [1]])
        with self.assertRaisesRegex(InputError, "cost row"):
            self.load(tree, "id,c\na,0\n")

    def test_negative_cost_rejected(self):
        tree = spec({"id": "r", "children": [{"id": "a"}]}, costs=[[0, -1], [1, 0]])
        with self.assertRaisesRegex(InputError, "negative"):
            self.load(tree, "id,c\na,0\n")

    def test_nonzero_diagonal_rejected(self):
        tree = spec({"id": "r", "children": [{"id": "a"}]}, costs=[[1, 1], [1, 0]])
        with self.assertRaisesRegex(InputError, "diagonal"):
            self.load(tree, "id,c\na,0\n")

    def test_too_many_states_rejected(self):
        states = ["s1", "s2", "s3", "s4", "s5", "s6", "s7"]
        costs = [[0 if i == j else 1 for j in range(7)] for i in range(7)]
        tree = spec({"id": "r", "children": [{"id": "a"}]}, states=states, costs=costs)
        with self.assertRaisesRegex(InputError, "1..6"):
            self.load(tree, "id,c\na,s1\n")


class ReconstructionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)

    def solve(self, tree_spec, csv_text):
        tree_path, csv_path = write_inputs(self.tmp.name, tree_spec, csv_text)
        problem = load_problem(tree_path, csv_path)
        return problem, reconstruct(problem)

    def test_multifurcation(self):
        # Root with three leaves; majority state wins at the root.
        tree = spec(
            {"id": "r", "children": [{"id": "a"}, {"id": "b"}, {"id": "c"}]}
        )
        problem, result = self.solve(tree, "id,c\na,1\nb,1\nc,0\n")
        self.assertEqual(result.assignments["r"], ["1"])
        self.assertEqual(result.total_score, 1.0)
        self.assertEqual(result.column_scores, [1.0])

    def test_unknown_leaf_is_free(self):
        tree = spec({"id": "r", "children": [{"id": "a"}, {"id": "b"}]})
        problem, result = self.solve(tree, "id,c\na,?\nb,0\n")
        self.assertEqual(result.assignments["a"], ["0"])
        self.assertEqual(result.total_score, 0.0)

    def test_known_leaf_is_fixed(self):
        tree = spec({"id": "r", "children": [{"id": "a"}, {"id": "b"}]})
        problem, result = self.solve(tree, "id,c\na,0\nb,1\n")
        self.assertEqual(result.assignments["a"], ["0"])
        self.assertEqual(result.assignments["b"], ["1"])
        self.assertEqual(result.total_score, 1.0)

    def test_directed_costs(self):
        # 0->1 costs 5, 1->0 costs 1: root should keep state 1.
        costs = [[0, 5], [1, 0]]
        tree = spec(
            {"id": "r", "children": [{"id": "a"}, {"id": "b"}]}, costs=costs
        )
        problem, result = self.solve(tree, "id,c\na,1\nb,0\n")
        self.assertEqual(result.assignments["r"], ["1"])
        self.assertEqual(result.total_score, 1.0)

    def test_root_tie_breaks_lexicographically(self):
        # Symmetric tie at the root: "A" < "B" must win regardless of the
        # order states are listed in the input.
        costs = [[0, 1], [1, 0]]
        tree = spec(
            {"id": "r", "children": [{"id": "a"}, {"id": "b"}]},
            states=("B", "A"),
            costs=costs,
        )
        problem, result = self.solve(tree, "id,c\na,A\nb,B\n")
        self.assertEqual(result.assignments["r"], ["A"])

    def test_child_order_does_not_matter(self):
        leaves = [{"id": "a"}, {"id": "b"}, {"id": "c"}]
        forward = spec({"id": "r", "children": leaves})
        reversed_tree = spec({"id": "r", "children": list(reversed(leaves))})
        csv_text = "id,c\na,0\nb,1\nc,1\n"
        _, first = self.solve(forward, csv_text)
        _, second = self.solve(reversed_tree, csv_text)
        self.assertEqual(first.assignments, second.assignments)
        self.assertEqual(first.total_score, second.total_score)

    def test_joint_backtrace_consistency(self):
        # Two characters sharing one tree; branch costs must re-add to the
        # per-column optima and to the total.
        tree = spec(
            {
                "id": "r",
                "children": [
                    {"id": "x", "children": [{"id": "a"}, {"id": "b"}]},
                    {"id": "y", "children": [{"id": "c"}, {"id": "d"}]},
                ],
            }
        )
        csv_text = "id,c1,c2\na,0,1\nb,0,1\nc,1,0\nd,1,?\n"
        problem, result = self.solve(tree, csv_text)
        for column, score in enumerate(result.column_scores):
            branch_sum = sum(b.per_character[column] for b in result.branch_costs)
            self.assertAlmostEqual(branch_sum, score)
        self.assertAlmostEqual(sum(result.column_scores), result.total_score)
        # Known leaves keep their observed states.
        self.assertEqual(result.assignments["c"], ["1", "0"])

    def test_cli_runs(self):
        tree = spec({"id": "r", "children": [{"id": "a"}, {"id": "b"}]})
        tree_path, csv_path = write_inputs(self.tmp.name, tree, "id,c\na,0\nb,1\n")
        self.assertEqual(main([str(tree_path), str(csv_path)]), 0)

    def test_cli_rejects_bad_input(self):
        tree = spec({"id": "r", "children": [{"id": "a"}]})
        tree_path, csv_path = write_inputs(self.tmp.name, tree, "id,c\na,X\n")
        self.assertEqual(main([str(tree_path), str(csv_path)]), 2)


if __name__ == "__main__":
    unittest.main()
