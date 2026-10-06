import math
import unittest

from composition_balance.core import (
    BalanceError,
    build_basis,
    clr,
    forward,
    inverse,
    pairwise_euclidean,
    parse_tree,
    replace_zeros,
    validate_and_close,
)

NAMES = ["a", "b", "c", "d"]
# Unbalanced tree: ((a,b),(c,d)) vs unbalanced (a,(b,(c,d)))
BALANCED = {"left": {"left": "a", "right": "b"},
            "right": {"left": "c", "right": "d"}}
UNBALANCED = {"left": "a",
              "right": {"left": "b",
                        "right": {"left": "c", "right": "d"}}}


class TestValidateClose(unittest.TestCase):
    def test_closes_to_one(self):
        row = validate_and_close(["1", "1", "2", "0"], NAMES)
        self.assertAlmostEqual(sum(row), 1.0)
        self.assertEqual(row, [0.25, 0.25, 0.5, 0.0])

    def test_rejects_negative_nonfinite_allzero(self):
        for bad in (["-1", "1", "1", "1"], ["nan", "1", "1", "1"],
                    ["inf", "1", "1", "1"], ["0", "0", "0", "0"],
                    ["x", "1", "1", "1"], ["1", "1"]):
            with self.assertRaises(BalanceError, msg=bad):
                validate_and_close(bad, NAMES)


class TestZeroReplacement(unittest.TestCase):
    def test_no_zeros_unchanged(self):
        row = [0.25, 0.25, 0.25, 0.25]
        self.assertEqual(replace_zeros(row, 1e-6), row)

    def test_replacement_keeps_closure_and_proportions(self):
        row = [0.5, 0.5, 0.0, 0.0]
        out = replace_zeros(row, 1e-3)
        self.assertAlmostEqual(sum(out), 1.0)
        self.assertEqual(out[2], 1e-3)
        self.assertEqual(out[3], 1e-3)
        self.assertAlmostEqual(out[0] / out[1], 1.0)
        self.assertAlmostEqual(out[0], (1 - 2e-3) / 2)

    def test_overflow_rejected(self):
        with self.assertRaises(BalanceError):
            replace_zeros([0.5, 0.5, 0.0, 0.0], 0.6)


class TestTree(unittest.TestCase):
    def test_leaf_mismatch_rejected(self):
        with self.assertRaises(BalanceError):
            parse_tree({"left": "a", "right": "b"}, NAMES)
        with self.assertRaises(BalanceError):
            parse_tree({"left": {"left": "a", "right": "a"},
                        "right": {"left": "c", "right": "d"}}, NAMES)

    def test_single_child_rejected(self):
        with self.assertRaises(BalanceError):
            parse_tree({"left": {"left": "a"}, "right": "b"}, ["a", "b"])

    def test_preorder_count(self):
        root = parse_tree(UNBALANCED, NAMES)
        self.assertEqual(len(root.internal_nodes_preorder()), 3)


class TestTransform(unittest.TestCase):
    def test_unbalanced_roundtrip(self):
        root = parse_tree(UNBALANCED, NAMES)
        comp = [0.1, 0.2, 0.3, 0.4]
        coords = forward(comp, root, NAMES)
        self.assertEqual(len(coords), 3)
        back = inverse(coords, root, NAMES)
        for got, want in zip(back, comp):
            self.assertAlmostEqual(got, want, places=12)

    def test_known_coordinate_value(self):
        root = parse_tree(BALANCED, NAMES)
        comp = [0.25, 0.25, 0.25, 0.25]
        coords = forward(comp, root, NAMES)
        self.assertTrue(all(abs(c) < 1e-15 for c in coords))
        comp2 = [0.5, 0.5 / 3, 0.5 / 3, 0.5 / 3]
        coords2 = forward(comp2, root, NAMES)
        self.assertAlmostEqual(coords2[0], 0.5 * math.log(3), places=12)

    def test_swap_branches_flips_sign_only(self):
        swapped = {"right": UNBALANCED["left"], "left": UNBALANCED["right"]}
        comp = [0.1, 0.2, 0.3, 0.4]
        root_a = parse_tree(UNBALANCED, NAMES)
        root_b = parse_tree(swapped, NAMES)
        ca = forward(comp, root_a, NAMES)
        cb = forward(comp, root_b, NAMES)
        self.assertAlmostEqual(ca[0], -cb[0], places=12)
        ra = inverse(ca, root_a, NAMES)
        rb = inverse(cb, root_b, NAMES)
        for x, y in zip(ra, rb):
            self.assertAlmostEqual(x, y, places=12)

    def test_distances_match_clr(self):
        root = parse_tree(UNBALANCED, NAMES)
        comps = [[0.1, 0.2, 0.3, 0.4], [0.4, 0.3, 0.2, 0.1],
                 [0.7, 0.1, 0.1, 0.1]]
        coords = [forward(c, root, NAMES) for c in comps]
        clrs = [clr(c) for c in comps]
        for (_, _, db), (_, _, dc) in zip(
            pairwise_euclidean(coords), pairwise_euclidean(clrs)
        ):
            self.assertAlmostEqual(db, dc, places=10)

    def test_inverse_overflow_safe(self):
        root = parse_tree(UNBALANCED, NAMES)
        out = inverse([1e4, -1e4, 5e3], root, NAMES)
        self.assertAlmostEqual(sum(out), 1.0)
        self.assertTrue(all(math.isfinite(v) and v >= 0 for v in out))

    def test_basis_orthonormal(self):
        root = parse_tree(UNBALANCED, NAMES)
        basis = build_basis(root, NAMES)
        for i, row in enumerate(basis):
            self.assertAlmostEqual(sum(v * v for v in row), 1.0, places=12)
            for other in basis[i + 1:]:
                self.assertAlmostEqual(
                    sum(a * b for a, b in zip(row, other)), 0.0, places=12)


if __name__ == "__main__":
    unittest.main()
