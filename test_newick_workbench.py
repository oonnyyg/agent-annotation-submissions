import io
import unittest

import newick_workbench as nw


def leaf_distances(root):
    names, matrix = nw.distance_matrix(root)
    return {a: {b: matrix[i][j] for j, b in enumerate(names)}
            for i, a in enumerate(names)}


class ParseTests(unittest.TestCase):
    def test_quoted_labels_and_doubled_quotes(self):
        root = nw.parse_newick("('it''s a':1,'b c':2,under_score:3,'keep_lit':4);")
        names = sorted(n.label for n in nw.leaves_of(root))
        self.assertEqual(names, ["b c", "it's a", "keep_lit", "under score"])

    def test_comments_and_multifurcation(self):
        root = nw.parse_newick("(a:1[comment], b:2, c:3, d:4)root;")
        self.assertEqual(len(root.children), 4)

    def test_internal_labels_optional(self):
        root = nw.parse_newick("((a:1,b:1)x:2,c:1);")
        self.assertEqual(root.children[0].label, "x")

    def test_missing_length_rejected(self):
        with self.assertRaises(nw.NewickError):
            nw.parse_newick("(a:1,b);")

    def test_negative_length_rejected(self):
        with self.assertRaises(nw.NewickError):
            nw.parse_newick("(a:1,b:-2);")

    def test_root_length_must_be_zero_or_absent(self):
        with self.assertRaises(nw.NewickError):
            nw.parse_newick("(a:1,b:1):3;")
        nw.parse_newick("(a:1,b:1):0;")

    def test_duplicate_and_empty_leaf_names_rejected(self):
        with self.assertRaises(nw.NewickError):
            nw.parse_newick("(a:1,a:2);")
        with self.assertRaises(nw.NewickError):
            nw.parse_newick("(a:1,:2);")

    def test_distance_matrix_no_double_counting(self):
        root = nw.parse_newick("((a:1,b:2):3,c:4);")
        d = leaf_distances(root)
        self.assertEqual(d["a"]["b"], 3.0)
        self.assertEqual(d["a"]["c"], 8.0)
        self.assertEqual(d["b"]["c"], 9.0)
        self.assertEqual(d["a"]["a"], 0.0)


class PruneTests(unittest.TestCase):
    def test_prune_preserves_distances_and_reports_labels(self):
        root = nw.parse_newick("((a:1,b:1)x:2,(c:1,d:4)y:2,e:1)r;")
        before = leaf_distances(root)
        root, removed = nw.prune(root, ["a", "c", "e"])
        after = leaf_distances(root)
        for x in after:
            for y in after:
                self.assertEqual(after[x][y], before[x][y])
        self.assertEqual(removed, ["x", "y"])
        self.assertEqual(sorted(after), ["a", "c", "e"])

    def test_prune_compresses_and_promotes_root(self):
        root = nw.parse_newick("(((a:1,b:1)x:1)y:1,c:1);")
        root, removed = nw.prune(root, ["a", "b"])
        self.assertIsNone(root.parent)
        self.assertEqual(sorted(n.label for n in nw.leaves_of(root)), ["a", "b"])
        d = leaf_distances(root)
        self.assertEqual(d["a"]["b"], 2.0)
        self.assertEqual(root.label, "x")  # surviving node keeps its label
        self.assertEqual(removed, ["y"])

    def test_prune_requires_two_known_leaves(self):
        root = nw.parse_newick("(a:1,b:1,c:1);")
        with self.assertRaises(nw.NewickError):
            nw.prune(root, ["a"])
        with self.assertRaises(nw.NewickError):
            nw.prune(root, ["a", "zzz"])


class MidpointTests(unittest.TestCase):
    def test_midpoint_inside_edge(self):
        root = nw.parse_newick("(a:5,b:1);")
        root = nw.midpoint_root(root)
        self.assertEqual(len(root.children), 2)
        d = leaf_distances(root)
        self.assertEqual(d["a"]["b"], 6.0)
        lengths = sorted(c.length for c in root.children)
        self.assertEqual(lengths, [2.0, 3.0])

    def test_midpoint_tie_break_and_zero_edges(self):
        root = nw.parse_newick("((a:0,b:0):0,c:0);")
        root = nw.midpoint_root(root)
        # All distances zero: pair (a, b) wins; the first halfway node
        # reached walking from a is a itself, so a keeps a zero-length
        # edge to the new root (placed at a's parent).
        self.assertEqual(len(root.children), 3)
        kids = {c.label: c.length for c in root.children if c.is_leaf}
        self.assertEqual(kids, {"a": 0.0, "b": 0.0})
        d = leaf_distances(root)
        self.assertEqual(d["a"]["c"], 0.0)

    def test_midpoint_at_leaf_gets_zero_edge(self):
        root = nw.parse_newick("(a:0,b:0,c:5);")
        root = nw.midpoint_root(root)
        # Farthest pair: (a, c) at distance 5; half = 2.5 inside an edge.
        d = leaf_distances(root)
        self.assertEqual(d["a"]["c"], 5.0)
        # All-zero tree: midpoint is leaf 'a' itself.
        root = nw.midpoint_root(nw.parse_newick("(a:0,b:0);"))
        kids = {c.label: c.length for c in root.children}
        self.assertEqual(kids, {"a": 0.0, "b": 0.0})

    def test_reroot_preserves_all_distances(self):
        root = nw.parse_newick("((a:1,b:2)x:3,(c:1,d:1)y:1,e:2);")
        before = leaf_distances(root)
        root = nw.midpoint_root(root)
        after = leaf_distances(root)
        self.assertEqual(before, after)

    def test_farthest_pair_tie_uses_sorted_names(self):
        # Pairs (a,d) and (b,d) tie at 4; lexicographically smaller pair wins.
        root = nw.parse_newick("((a:1,b:1):1,(c:1,d:1):1);")
        root = nw.midpoint_root(root)
        d = leaf_distances(root)
        self.assertEqual(d["a"]["d"], 4.0)


class ExportTests(unittest.TestCase):
    def test_deterministic_order_and_quoting(self):
        root = nw.parse_newick("(z:1,'a b':1,m:1);")
        text = nw.to_newick(root)
        self.assertEqual(text, "('a b':1,m:1,z:1);")

    def test_quote_roundtrip_with_single_quote(self):
        root = nw.parse_newick("('it''s':1,b:1);")
        text = nw.to_newick(root)
        again = nw.parse_newick(text)
        names = sorted(n.label for n in nw.leaves_of(again))
        self.assertEqual(names, ["b", "it's"])

    def test_csv_output(self):
        root = nw.parse_newick("(a:1,b:2);")
        names, matrix = nw.distance_matrix(root)
        buf = io.StringIO()
        nw.matrix_csv(names, matrix, buf)
        self.assertEqual(buf.getvalue(), ",a,b\na,0,3\nb,3,0\n")

    def test_full_pipeline_distance_preservation(self):
        root = nw.parse_newick("((a:1,b:2)x:3,(c:1,d:1)y:1,e:2);")
        before = leaf_distances(root)
        kept = ["a", "b", "c", "e"]
        root, _ = nw.prune(root, kept)
        mid = leaf_distances(root)
        root = nw.midpoint_root(root)
        after = leaf_distances(root)
        for x in kept:
            for y in kept:
                self.assertEqual(before[x][y], mid[x][y])
        self.assertEqual(mid, after)
        # Reparse exported tree and confirm distances survive a round trip.
        reparsed = nw.parse_newick(nw.to_newick(root))
        self.assertEqual(leaf_distances(reparsed), after)


if __name__ == "__main__":
    unittest.main()
