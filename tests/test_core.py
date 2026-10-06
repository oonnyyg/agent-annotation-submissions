import math
import unittest

from composition_balance import core

NAMES = ["a", "b", "c", "d"]
# Unbalanced tree: ((a,b),(c,d)) would be balanced; use (a,(b,(c,d))).
UNBALANCED = {"left": "a", "right": {"left": "b", "right": {"left": "c", "right": "d"}}}


class CloseAndReplaceTests(unittest.TestCase):
    def test_close_scales_rows(self):
        self.assertEqual(core.close_rows([[1.0, 1.0, 2.0]]), [[0.25, 0.25, 0.5]])

    def test_close_rejects_bad_rows(self):
        for bad in ([[0.0, 0.0]], [[-1.0, 2.0]], [[float("nan"), 1.0]], [[float("inf"), 1.0]]):
            with self.assertRaises(core.CompositionError):
                core.close_rows(bad)

    def test_replace_zeros_keeps_sum_one(self):
        rows, counts = core.replace_zeros([[0.0, 0.5, 0.5, 0.0]], 1e-3)
        self.assertEqual(counts, [2])
        self.assertAlmostEqual(math.fsum(rows[0]), 1.0, places=15)
        self.assertEqual(rows[0][0], 1e-3)
        self.assertAlmostEqual(rows[0][1], rows[0][2])
        self.assertLess(rows[0][1], 0.5)

    def test_replace_zeros_untouched_without_zeros(self):
        original = [[0.1, 0.2, 0.3, 0.4]]
        rows, counts = core.replace_zeros(original, 1e-6)
        self.assertEqual(rows, original)
        self.assertEqual(counts, [0])

    def test_replace_zeros_rejects_large_delta(self):
        with self.assertRaises(core.CompositionError):
            core.replace_zeros([[0.0, 0.0, 1.0]], 0.6)


class TreeTests(unittest.TestCase):
    def test_unbalanced_leaf_counts(self):
        root = core.parse_tree(UNBALANCED, NAMES)
        nodes = root.internal_preorder()
        self.assertEqual(len(nodes), 3)
        self.assertEqual([len(n.left.leaves()) for n in nodes], [1, 1, 1])
        self.assertEqual([len(n.right.leaves()) for n in nodes], [3, 2, 1])

    def test_mismatched_leaves_rejected(self):
        with self.assertRaises(core.TreeError):
            core.parse_tree(UNBALANCED, ["a", "b", "c", "x"])

    def test_invalid_internal_node_rejected(self):
        with self.assertRaises(core.TreeError):
            core.parse_tree({"left": "a"}, ["a"])
        with self.assertRaises(core.TreeError):
            core.parse_tree({"left": "a", "right": "b", "extra": "c"}, ["a", "b", "c"])

    def test_balance_values_unbalanced(self):
        root = core.parse_tree(UNBALANCED, NAMES)
        row = [0.1, 0.2, 0.3, 0.4]
        coords = core.balance_coordinates([row], root, NAMES)[0]
        g_bcd = (0.2 * 0.3 * 0.4) ** (1 / 3)
        self.assertAlmostEqual(coords[0], math.sqrt(3 / 4) * math.log(0.1 / g_bcd))
        g_cd = math.sqrt(0.3 * 0.4)
        self.assertAlmostEqual(coords[1], math.sqrt(2 / 3) * math.log(0.2 / g_cd))
        self.assertAlmostEqual(coords[2], math.sqrt(1 / 2) * math.log(0.3 / 0.4))


def _swap(node):
    if isinstance(node, str):
        return node
    if "name" in node:
        return node
    return {"left": _swap(node["right"]), "right": _swap(node["left"])}


class RoundTripTests(unittest.TestCase):
    def setUp(self):
        self.root = core.parse_tree(UNBALANCED, NAMES)
        self.rows = core.close_rows([[1.0, 2.0, 3.0, 4.0], [0.0, 5.0, 1.0, 2.0]])
        self.rows, _ = core.replace_zeros(self.rows, 1e-6)

    def test_basis_is_orthonormal(self):
        basis = core.orthonormal_basis(self.root, NAMES)
        for i, row in enumerate(basis):
            self.assertAlmostEqual(math.fsum(v * v for v in row), 1.0, places=12)
            for j in range(i):
                dot = math.fsum(a * b for a, b in zip(row, basis[j]))
                self.assertAlmostEqual(dot, 0.0, places=12)

    def test_round_trip(self):
        coords = core.balance_coordinates(self.rows, self.root, NAMES)
        rebuilt = core.inverse_balances(coords, self.root, NAMES)
        for original, recovered in zip(self.rows, rebuilt):
            self.assertAlmostEqual(math.fsum(recovered), 1.0, places=12)
            for a, b in zip(original, recovered):
                self.assertAlmostEqual(a, b, places=12)

    def test_inverse_avoids_overflow(self):
        rebuilt = core.inverse_balances([[1e4, -1e4, 0.0]], self.root, NAMES)
        self.assertAlmostEqual(math.fsum(rebuilt[0]), 1.0, places=12)
        self.assertTrue(all(math.isfinite(v) for v in rebuilt[0]))
        rebuilt = core.inverse_balances([[100.0, -100.0, 0.0]], self.root, NAMES)
        self.assertTrue(all(v > 0.0 for v in rebuilt[0]))

    def test_distances_match_clr(self):
        coords = core.balance_coordinates(self.rows, self.root, NAMES)
        d_balance = core.pairwise_distances(coords)[0][1]
        d_clr = core.pairwise_distances(core.clr_coordinates(self.rows))[0][1]
        self.assertAlmostEqual(d_balance, d_clr, places=12)

    def test_swapped_tree_same_reconstruction_and_distances(self):
        swapped_root = core.parse_tree(_swap(UNBALANCED), NAMES)
        coords_a = core.balance_coordinates(self.rows, self.root, NAMES)
        coords_b = core.balance_coordinates(self.rows, swapped_root, NAMES)
        for ra, rb in zip(coords_a, coords_b):
            for a, b in zip(ra, rb):
                self.assertAlmostEqual(a, -b, places=12)
        rec_a = core.inverse_balances(coords_a, self.root, NAMES)
        rec_b = core.inverse_balances(coords_b, swapped_root, NAMES)
        for ra, rb in zip(rec_a, rec_b):
            for a, b in zip(ra, rb):
                self.assertAlmostEqual(a, b, places=12)
        self.assertAlmostEqual(
            core.pairwise_distances(coords_a)[0][1],
            core.pairwise_distances(coords_b)[0][1],
            places=12,
        )


if __name__ == "__main__":
    unittest.main()
