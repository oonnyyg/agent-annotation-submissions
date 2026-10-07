import json
import math
import tempfile
import unittest
from pathlib import Path

from ancestral_parsimony.data import load_costs, load_traits, validate_observations
from ancestral_parsimony.errors import InputError
from ancestral_parsimony.sankoff import sankoff_column
from ancestral_parsimony.tree import Tree, load_tree


def write(tmp, name, text):
    path = Path(tmp) / name
    path.write_text(text, encoding="utf-8")
    return str(path)


UNIT = [[0, 1], [1, 0]]


def make_tree(children, root):
    return Tree(children=children, root=root,
                leaves=sorted(n for n in children if not children[n]))


class MultifurcationTest(unittest.TestCase):
    def test_multifurcating_root(self):
        # Root with three leaves: a, a, b -> root should be 'a', cost 1.
        tree = make_tree({1: [2, 3, 4], 2: [], 3: [], 4: []}, 1)
        obs = {2: "a", 3: "a", 4: "b"}
        assignment, total, branch = sankoff_column(tree, obs, ["a", "b"], UNIT)
        self.assertEqual(total, 1)
        self.assertEqual(assignment[1], "a")
        self.assertEqual(sum(branch.values()), total)

    def test_unknown_leaf_free(self):
        # Unknown leaf may take any state: zero-cost assignment exists.
        tree = make_tree({1: [2, 3], 2: [], 3: []}, 1)
        obs = {2: "a", 3: "?"}
        assignment, total, _ = sankoff_column(tree, obs, ["a", "b"], UNIT)
        self.assertEqual(total, 0)
        self.assertEqual(assignment[3], "a")

    def test_known_leaf_fixed(self):
        tree = make_tree({1: [2], 2: []}, 1)
        obs = {2: "b"}
        assignment, total, _ = sankoff_column(tree, obs, ["a", "b"], UNIT)
        self.assertEqual(assignment[2], "b")
        self.assertEqual(total, 0)  # root is free and takes 'b'


class DirectedCostTest(unittest.TestCase):
    def test_asymmetric_costs(self):
        # a->b costs 5, b->a costs 1. Leaf fixed to 'b'; root is free, so
        # the optimum keeps everything at 'b' with zero cost.
        matrix = [[0, 5], [1, 0]]
        tree = make_tree({1: [2], 2: []}, 1)
        obs = {2: "b"}
        assignment, total, _ = sankoff_column(tree, obs, ["a", "b"], matrix)
        self.assertEqual(total, 0)
        self.assertEqual(assignment, {1: "b", 2: "b"})

    def test_direction_matters(self):
        # With leaves forcing root='a' subtree and a 'b' leaf, the cheap
        # direction b->a=1 vs a->b=5 changes the optimum.
        matrix = [[0, 5], [1, 0]]
        tree = make_tree({1: [2, 3], 2: [], 3: []}, 1)
        obs = {2: "a", 3: "b"}
        _, total, _ = sankoff_column(tree, obs, ["a", "b"], matrix)
        # root=a: 0 + 5 = 5; root=b: 1 + 0 = 1 -> total 1
        self.assertEqual(total, 1)


class TieBreakTest(unittest.TestCase):
    def test_root_lexicographic(self):
        # All leaves unknown: every root state optimal; pick lex smallest.
        tree = make_tree({1: [2, 3], 2: [], 3: []}, 1)
        obs = {2: "?", 3: "?"}
        assignment, total, _ = sankoff_column(tree, obs, ["b", "a"], UNIT)
        self.assertEqual(assignment[1], "a")
        self.assertEqual(total, 0)

    def test_child_order_irrelevant(self):
        # Same tree with children listed in different orders must agree.
        obs = {2: "a", 3: "?", 4: "b"}
        t1 = make_tree({1: [2, 3, 4], 2: [], 3: [], 4: []}, 1)
        t2 = make_tree({1: [4, 3, 2], 2: [], 3: [], 4: []}, 1)
        a1 = sankoff_column(t1, obs, ["a", "b"], UNIT)
        a2 = sankoff_column(t2, obs, ["a", "b"], UNIT)
        self.assertEqual(a1, a2)

    def test_joint_backtrack_consistency(self):
        # Deep tree where independent per-node minima would be incompatible:
        # internal node 2 has equal min cost for a/b, but given root state
        # the constrained choice must match the parent.
        matrix = [[0, 2], [3, 0]]
        tree = make_tree({1: [2], 2: [3, 4], 3: [], 4: []}, 1)
        obs = {3: "a", 4: "b"}
        assignment, total, branch = sankoff_column(tree, obs, ["a", "b"], matrix)
        # Verify assignment actually attains the DP optimum by re-summing.
        recomputed = sum(matrix[["a", "b"].index(assignment[p])]
                         [["a", "b"].index(assignment[c])]
                         for (p, c) in branch)
        self.assertEqual(recomputed, total)
        # Brute-force check over all assignments.
        import itertools
        best = math.inf
        for s1, s2 in itertools.product("ab", repeat=2):
            cost = (matrix["ab".index(s1)]["ab".index(s2)]
                    + matrix["ab".index(s2)][0]
                    + matrix["ab".index(s2)][1])
            best = min(best, cost)
        self.assertEqual(total, best)
