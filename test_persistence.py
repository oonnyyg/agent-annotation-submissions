import json
import os
import random
import tempfile
import unittest

from io_utils import build_report, load_points, load_thresholds
from persistence import INFINITY, compute_persistence


def write(tmp, name, text):
    path = os.path.join(tmp, name)
    with open(path, "w", encoding="utf-8") as fh:
        fh.write(text)
    return path


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.mkdtemp()

    def test_duplicate_id_rejected(self):
        path = write(self.tmp, "p.csv", "a,0,0\na,1,1\n")
        with self.assertRaises(ValueError):
            load_points(path)

    def test_non_finite_rejected(self):
        path = write(self.tmp, "p.csv", "a,0,nan\n")
        with self.assertRaises(ValueError):
            load_points(path)

    def test_point_count_bounds(self):
        with self.assertRaises(ValueError):
            load_points(write(self.tmp, "p.csv", ""))
        rows = "".join(f"p{i},{i},0\n" for i in range(301))
        with self.assertRaises(ValueError):
            load_points(write(self.tmp, "q.csv", rows))

    def test_same_coordinates_allowed(self):
        path = write(self.tmp, "p.csv", "a,1,1\nb,1,1\n")
        self.assertEqual(len(load_points(path)), 2)

    def test_negative_threshold_rejected(self):
        path = write(self.tmp, "t.json", "[-1]")
        with self.assertRaises(ValueError):
            load_thresholds(path)


class PersistenceTests(unittest.TestCase):
    def test_single_point(self):
        result = compute_persistence({"only": (0.0, 0.0)})
        self.assertEqual(result.intervals(), [("only", 0.0, INFINITY)])
        self.assertEqual(result.components_at(0.0), [["only"]])
        report = build_report(result, [0.0, 5.0])
        self.assertEqual([q["component_count"] for q in report["queries"]], [1, 1])

    def test_coincident_points_zero_length_interval(self):
        result = compute_persistence({"a": (1.0, 1.0), "b": (1.0, 1.0)})
        intervals = dict((pid, death) for pid, _, death in result.intervals())
        self.assertEqual(intervals, {"a": INFINITY, "b": 0.0})
        self.assertEqual(result.components_at(0.0), [["a", "b"]])

    def test_left_closed_right_open(self):
        # a--b distance 2, b--c distance 2: batch at d=2 merges all three.
        pts = {"a": (0.0, 0.0), "b": (2.0, 0.0), "c": (4.0, 0.0)}
        result = compute_persistence(pts)
        self.assertEqual(result.components_at(1.999), [["a"], ["b"], ["c"]])
        self.assertEqual(result.components_at(2.0), [["a", "b", "c"]])
        for merge in result.merges:
            self.assertEqual(merge.survivor, "a")

    def test_equidistant_batch_points_to_final_representative(self):
        # Equilateral-ish chain: all three pairs at distance 1.
        pts = {"b": (0.0, 0.0), "a": (1.0, 0.0), "c": (0.5, 0.8660254037844386)}
        # force exact equidistance by overriding coordinates
        pts["c"] = (0.5, (1 - 0.25) ** 0.5)
        result = compute_persistence(pts)
        self.assertEqual(len(result.merges), 2)
        self.assertTrue(all(m.survivor == "a" for m in result.merges))
        self.assertEqual({m.died for m in result.merges}, {"b", "c"})

    def test_input_order_independence(self):
        pts = {"a": (0.0, 0.0), "b": (1.0, 0.0), "c": (5.0, 0.0),
               "d": (6.0, 0.0)}
        base = compute_persistence(pts)
        for seed in range(10):
            items = list(pts.items())
            random.Random(seed).shuffle(items)
            shuffled = compute_persistence(dict(items))
            self.assertEqual(base.intervals(), shuffled.intervals())
            self.assertEqual(
                [(m.distance, m.died, m.survivor) for m in base.merges],
                [(m.distance, m.died, m.survivor) for m in shuffled.merges])

    def test_component_count_matches_alive_intervals(self):
        pts = {f"p{i}": (float(i * 3), float((i % 3) ** 2)) for i in range(8)}
        result = compute_persistence(pts)
        report = build_report(result, [0.0, 1.5, 3.0, 100.0])
        for query in report["queries"]:
            alive = sum(
                1 for row in report["intervals"]
                if row["death"] == INFINITY or row["death"] > query["threshold"])
            self.assertEqual(query["component_count"], alive)
            members = sorted(pid for g in query["components"] for pid in g)
            self.assertEqual(members, sorted(pts))


if __name__ == "__main__":
    unittest.main()
