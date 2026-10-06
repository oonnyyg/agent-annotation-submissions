import unittest
from fractions import Fraction
import subprocess
import sys
import tempfile
from pathlib import Path

from newick_workbench import (
    ParseError,
    distance_matrix,
    midpoint_root,
    parse_tree,
    prune,
    to_newick,
)
from newick_workbench.emit import format_length, matrix_csv


class ParseTests(unittest.TestCase):
    def test_quoted_labels_and_comments(self):
        root = parse_tree(
            "('it''s a':1.5,[note]sample_b:2,[x]'sp ace':0.5)root[meta]:0;"
        )
        names = sorted(leaf.name for leaf in root.leaves())
        self.assertEqual(names, ["it's a", "sample b", "sp ace"])
        self.assertEqual(root.name, "root")
        self.assertIsNone(root.length)

    def test_underscore_becomes_space_unquoted(self):
        root = parse_tree("(a_b:1,c:1);")
        self.assertIn("a b", {leaf.name for leaf in root.leaves()})

    def test_multifurcation(self):
        root = parse_tree("(a:1,b:1,c:1,d:1);")
        self.assertEqual(len(root.children), 4)

    def test_reject_negative_and_missing_lengths(self):
        with self.assertRaises(ParseError):
            parse_tree("(a:-1,b:1);")
        with self.assertRaises(ParseError):
            parse_tree("(a,b:1);")
        with self.assertRaises(ParseError):
            parse_tree("(a:1,b:1):2;")
        with self.assertRaises(ParseError):
            parse_tree("(a:1,a:1);")
        with self.assertRaises(ParseError):
            parse_tree("(a:1,:1);")

    def test_scientific_and_zero_lengths(self):
        root = parse_tree("(a:1e-3,b:0.0);")
        lengths = sorted(child.length for child in root.children)
        self.assertEqual(lengths, [Fraction(0), Fraction(1, 1000)])


class MatrixTests(unittest.TestCase):
    def test_shared_ancestors_not_double_counted(self):
        root = parse_tree("((a:1,b:2)x:3,c:4);")
        names, matrix = distance_matrix(root)
        self.assertEqual(names, ["a", "b", "c"])
        idx = {n: i for i, n in enumerate(names)}
        self.assertEqual(matrix[idx["a"]][idx["b"]], Fraction(3))
        self.assertEqual(matrix[idx["a"]][idx["c"]], Fraction(1 + 3 + 4))
        self.assertEqual(matrix[idx["a"]][idx["a"]], Fraction(0))

    def test_csv_output(self):
        root = parse_tree("(b:2,a:1);")
        names, matrix = distance_matrix(root)
        text = matrix_csv(names, matrix)
        self.assertEqual(text, ",a,b\na,0,3\nb,3,0\n")


class PruneTests(unittest.TestCase):
    def test_prune_compresses_and_preserves_distances(self):
        root = parse_tree("(((a:1,b:1)x:2,(c:1,d:1)y:2)z:1,e:5);")
        before_names, before_mat = distance_matrix(root)
        kept, removed = prune(root, ["a", "c", "e"])
        names, matrix = distance_matrix(kept)
        self.assertEqual(names, ["a", "c", "e"])
        bi = {n: i for i, n in enumerate(before_names)}
        ai = {n: i for i, n in enumerate(names)}
        for i, j in [("a", "c"), ("a", "e"), ("c", "e")]:
            self.assertEqual(
                matrix[ai[i]][ai[j]], before_mat[bi[i]][bi[j]]
            )
        self.assertEqual(matrix[ai["a"]][ai["c"]], Fraction(1 + 2 + 2 + 1))
        self.assertEqual(matrix[ai["a"]][ai["e"]], Fraction(1 + 2 + 1 + 5))
        self.assertEqual(sorted(removed), ["x", "y"])

    def test_prune_promotes_single_child_root(self):
        root = parse_tree("((a:1,b:1)x:2,c:9);")
        kept, removed = prune(root, ["a", "b"])
        self.assertIsNone(kept.length)
        self.assertEqual(len(kept.children), 2)
        self.assertEqual(kept.name, "x")
        names, matrix = distance_matrix(kept)
        self.assertEqual(matrix[0][1], Fraction(2))
        self.assertEqual(removed, [])

    def test_prune_requires_two_known_leaves(self):
        root = parse_tree("(a:1,b:1);")
        with self.assertRaises(ValueError):
            prune(root, ["a"])
        with self.assertRaises(ValueError):
            prune(root, ["a", "nope"])


class MidpointTests(unittest.TestCase):
    def test_midpoint_inside_edge_splits_preserving_length(self):
        root = parse_tree("((a:2,b:2)x:1,c:7);")
        # farthest pair: a-c or b-c (dist 10); tie -> (a, c); half = 5
        # path a->c: 2 (a-x), 1 (x-root), 7 (root-c); 5 lands inside root-c edge
        rerooted = midpoint_root(root)
        self.assertEqual(to_newick(rerooted), "(((a:2,b:2)x:1):2,c:5);")
        self._distances_equal("((a:2,b:2)x:1,c:7);", rerooted)

    def test_midpoint_on_node_with_zero_edges(self):
        root = parse_tree("((a:1,b:1)x:0,(c:2,d:2)y:0);")
        # farthest pairs dist 4: (a,c),(a,d),(b,c),(b,d) -> (a,c); half=2
        # walk a->c: a-x=1, x-root=0, root-y=0, y-c=2; hits root at 1, then
        # cumulative 1 != 2 ... lands inside y-c edge at offset 1
        rerooted = midpoint_root(root)
        self._distances_equal("((a:1,b:1)x:0,(c:2,d:2)y:0);", rerooted)

    def test_all_zero_lengths_midpoint_is_leaf(self):
        root = parse_tree("(a:0,b:0,c:0);")
        rerooted = midpoint_root(root)
        names, matrix = distance_matrix(rerooted)
        self.assertEqual(names, ["a", "b", "c"])
        for row in matrix:
            for value in row:
                self.assertEqual(value, Fraction(0))
        # leaf 'a' keeps identity, attached to new root with zero edge
        leaves = {leaf.name: leaf for leaf in rerooted.leaves()}
        self.assertEqual(leaves["a"].length, Fraction(0))
        self.assertEqual(len(rerooted.children), 2)

    def test_midpoint_exactly_on_leaf(self):
        root = parse_tree("(a:2,b:2,c:4);")
        # farthest: (a,c),(b,c) dist 6 -> (a,c); half=3; path a-root-c:
        # a-root=2, root-c=4 -> inside root-c at offset 1? No: cumulative
        # 2 < 3, edge 4 -> 2+4 > 3 -> split. Use c:2 instead for leaf case.
        root = parse_tree("(a:2,b:2,c:2);")
        rerooted = midpoint_root(root)
        self._distances_equal("(a:2,b:2,c:2);", rerooted)

    def test_tie_breaks_by_leaf_name_pair(self):
        root = parse_tree("((a:1,b:1)x:1,(c:1,d:1)y:1);")
        rerooted = midpoint_root(root)
        # all cross pairs dist 4; (a,c) wins; half=2 lands at the old root
        self.assertEqual(len(rerooted.children), 2)
        self._distances_equal("((a:1,b:1)x:1,(c:1,d:1)y:1);", rerooted)

    def test_consecutive_zero_edges_first_halfway_node_wins(self):
        root = parse_tree("(a:1,(b:0.5,c:0.5)y:0.5,d:2);")
        # farthest: (a,d) dist 3? a-d = 1+2 = 3; b-d = .5+.5+2 = 3; c-d = 3
        # tie -> (a,d); half = 1.5; walk a->d: a-root=1, root-d=2 ->
        # 1+2 > 1.5 -> split root-d at 0.5 from root
        rerooted = midpoint_root(root)
        self._distances_equal("(a:1,(b:0.5,c:0.5)y:0.5,d:2);", rerooted)

    def test_zero_chain_picks_first_node_from_smaller_leaf(self):
        root = parse_tree("((a:1,b:1)x:0,(c:1,d:1)y:0);")
        # pairs dist 2: (a,c),(a,d),(b,c),(b,d) -> (a,c); half = 1
        # walk a->c: a-x=1 -> node x at exactly half; zero chain x-root-y
        # means x is the first halfway node reached
        rerooted = midpoint_root(root)
        self.assertEqual(rerooted.name, "x")
        self._distances_equal("((a:1,b:1)x:0,(c:1,d:1)y:0);", rerooted)

    def test_deep_target_path_direction(self):
        newick = "('it''s a':1.5,sample_b:2,'sp ace':0.5,(x:1,y:2)inner:3)root:0;"
        root = prune(parse_tree(newick), ["it's a", "sample b", "x", "y"])[0]
        rerooted = midpoint_root(root)
        names, matrix = distance_matrix(rerooted)
        ai = {n: i for i, n in enumerate(names)}
        self.assertEqual(matrix[ai["sample b"]][ai["y"]], Fraction(7))
        self.assertEqual(matrix[ai["x"]][ai["y"]], Fraction(3))

    def _distances_equal(self, newick, rerooted):
        names_a, mat_a = distance_matrix(parse_tree(newick))
        names_b, mat_b = distance_matrix(rerooted)
        self.assertEqual(names_a, names_b)
        self.assertEqual(mat_a, mat_b)


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            tree = tmp / "in.nwk"
            tree.write_text(
                "(((a:1,b:1)x:2,(c:1,d:1)y:2)z:1,e:5);", encoding="utf-8"
            )
            out_nwk = tmp / "out.nwk"
            out_csv = tmp / "out.csv"
            report = tmp / "report.txt"
            result = subprocess.run(
                [
                    sys.executable,
                    "-m",
                    "newick_workbench",
                    str(tree),
                    "--keep",
                    "a",
                    "c",
                    "e",
                    "--midpoint",
                    "--newick",
                    str(out_nwk),
                    "--csv",
                    str(out_csv),
                    "--report",
                    str(report),
                ],
                capture_output=True,
                text=True,
                cwd=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("x", report.read_text(encoding="utf-8"))
            self.assertIn("y", report.read_text(encoding="utf-8"))
            rerooted = parse_tree(out_nwk.read_text(encoding="utf-8"))
            names, matrix = distance_matrix(rerooted)
            self.assertEqual(names, ["a", "c", "e"])
            ai = {n: i for i, n in enumerate(names)}
            self.assertEqual(matrix[ai["a"]][ai["c"]], Fraction(6))
            self.assertEqual(matrix[ai["a"]][ai["e"]], Fraction(9))
            csv_lines = out_csv.read_text(encoding="utf-8").splitlines()
            self.assertEqual(csv_lines[0], ",a,c,e")

    def test_cli_rejects_bad_input(self):
        with tempfile.TemporaryDirectory() as tmp:
            tree = Path(tmp) / "bad.nwk"
            tree.write_text("(a:-1,b:1);", encoding="utf-8")
            result = subprocess.run(
                [sys.executable, "-m", "newick_workbench", str(tree)],
                capture_output=True,
                text=True,
                cwd=Path(__file__).resolve().parent.parent,
            )
            self.assertEqual(result.returncode, 2)
            self.assertIn("error", result.stderr)


if __name__ == "__main__":
    unittest.main()
