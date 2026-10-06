import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from pair_distribution.analysis import (
    analyze_pair,
    check_r_max,
    frame_histogram,
    min_image,
)
from pair_distribution.trajectory import (
    ConfigError,
    Frame,
    load_trajectory,
)


def make_frame(box, particles):
    ids = [p[0] for p in particles]
    types = [p[1] for p in particles]
    pos = [tuple(p[2]) for p in particles]
    return Frame(list(box), ids, types, pos)


class MinImageTests(unittest.TestCase):
    def test_wraps_across_boundary(self):
        d = min_image((9.5, -9.5, 0.0), (10.0, 10.0, 10.0))
        self.assertEqual(d, (-0.5, 0.5, 0.0))

    def test_histogram_counts_pbc_neighbor(self):
        frame = make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (0.1, 5.0, 5.0)), (2, "A", (9.9, 5.0, 5.0))],
        )
        hist = frame_histogram(frame, [0, 1], [0, 1], 10, 5.0, True)
        # distance 0.2 falls in bin 0 (dr=0.5), ordered pairs => 2
        self.assertEqual(hist[0], 2)
        self.assertEqual(sum(hist), 2)


class HistogramEdgeTests(unittest.TestCase):
    def test_bins_left_closed_right_open(self):
        frame = make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (0.0, 0.0, 0.0)), (2, "B", (1.0, 0.0, 0.0))],
        )
        # dr = 0.5, r = 1.0 -> bin 2, not bin 1
        hist = frame_histogram(frame, [0], [1], 10, 5.0, False)
        self.assertEqual(hist[2], 1)
        self.assertEqual(sum(hist), 1)

    def test_r_max_excluded(self):
        frame = make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (0.0, 0.0, 0.0)), (2, "B", (5.0, 0.0, 0.0))],
        )
        hist = frame_histogram(frame, [0], [1], 10, 5.0, False)
        self.assertEqual(sum(hist), 0)


class SameSpeciesTests(unittest.TestCase):
    def test_ordered_pairs_double_counted(self):
        frame = make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (1.0, 5.0, 5.0)), (2, "A", (2.0, 5.0, 5.0))],
        )
        hist = frame_histogram(frame, [0, 1], [0, 1], 10, 5.0, True)
        self.assertEqual(sum(hist), 2)  # (1->2) and (2->1)

    def test_fewer_than_two_same_species_gives_empty(self):
        frames = [make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (1.0, 5.0, 5.0)), (2, "B", (2.0, 5.0, 5.0))],
        )]
        res = analyze_pair(frames, ("A", "A"), 10, 5.0)
        self.assertTrue(res["per_frame"][0]["empty"])
        self.assertEqual(res["avg_g"], [0.0] * 10)
        self.assertEqual(res["avg_cn"], [0.0] * 10)

    def test_missing_species_gives_empty(self):
        frames = [make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (1.0, 5.0, 5.0))],
        )]
        res = analyze_pair(frames, ("A", "C"), 10, 5.0)
        self.assertEqual(res["avg_g"], [0.0] * 10)
        self.assertEqual(res["avg_cn"], [0.0] * 10)


class NormalizationTests(unittest.TestCase):
    def test_equal_weight_frame_average_varying_box(self):
        # Same pair distance in two frames with different volumes:
        # per-frame g differs by the volume ratio; the average must be
        # the equal-weight mean of the two per-frame values.
        f1 = make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (0.0, 0.0, 0.0)), (2, "B", (1.0, 0.0, 0.0))],
        )
        f2 = make_frame(
            (20.0, 20.0, 20.0),
            [(1, "A", (0.0, 0.0, 0.0)), (2, "B", (1.0, 0.0, 0.0))],
        )
        res = analyze_pair([f1, f2], ("A", "B"), 10, 5.0)
        dr = 0.5
        shell = 4.0 / 3.0 * math.pi * ((3 * dr) ** 3 - (2 * dr) ** 3)
        g1 = 1.0 / (1 * (1 / 1000.0) * shell)
        g2 = 1.0 / (1 * (1 / 8000.0) * shell)
        self.assertAlmostEqual(res["per_frame"][0]["g"][2], g1, places=9)
        self.assertAlmostEqual(res["per_frame"][1]["g"][2], g2, places=9)
        self.assertAlmostEqual(res["avg_g"][2], (g1 + g2) / 2, places=9)
        # CN within r_max is 1 in both frames.
        self.assertAlmostEqual(res["avg_cn"][-1], 1.0, places=9)

    def test_grid_coordination_number(self):
        # 3x3x3 grid of A, spacing 10/3: shells at 10/3 (6 neighbors)
        # and sqrt(2)*10/3 (12 neighbors) lie inside r_max = 5.
        particles = []
        pid = 0
        for i in range(3):
            for j in range(3):
                for k in range(3):
                    pid += 1
                    particles.append(
                        (pid, "A", (i * 10 / 3, j * 10 / 3, k * 10 / 3))
                    )
        frame = make_frame((10.0,) * 3, particles)
        res = analyze_pair([frame], ("A", "A"), 25, 5.0)
        self.assertAlmostEqual(res["avg_cn"][-1], 18.0, places=9)

    def test_frame_order_does_not_change_average(self):
        f1 = make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (1.0, 1.0, 1.0)), (2, "B", (2.0, 1.0, 1.0))],
        )
        f2 = make_frame(
            (12.0, 12.0, 12.0),
            [(1, "A", (1.0, 1.0, 1.0)), (2, "B", (3.0, 1.0, 1.0))],
        )
        r1 = analyze_pair([f1, f2], ("A", "B"), 10, 5.0)
        r2 = analyze_pair([f2, f1], ("A", "B"), 10, 5.0)
        self.assertEqual(r1["avg_g"], r2["avg_g"])
        self.assertEqual(r1["avg_cn"], r2["avg_cn"])

    def test_cross_pair_density_uses_target_count(self):
        frame = make_frame(
            (10.0, 10.0, 10.0),
            [(1, "A", (0.0, 0.0, 0.0)), (2, "B", (1.0, 0.0, 0.0))],
        )
        res = analyze_pair([frame], ("A", "B"), 10, 5.0)
        dr = 0.5
        shell = 4.0 / 3.0 * math.pi * ((2 * dr + dr) ** 3 - (2 * dr) ** 3)
        expected = 1.0 / (1 * (1 / 1000.0) * shell)
        self.assertAlmostEqual(res["avg_g"][2], expected, places=9)

    def test_r_max_limit_uses_shortest_edge_any_frame(self):
        frames = [
            make_frame((10.0, 10.0, 10.0), [(1, "A", (0, 0, 0))]),
            make_frame((8.0, 10.0, 10.0), [(1, "A", (0, 0, 0))]),
        ]
        check_r_max(frames, 4.0)
        with self.assertRaises(ConfigError):
            check_r_max(frames, 4.001)


class ValidationTests(unittest.TestCase):
    def _write_jsonl(self, lines):
        fd, path = tempfile.mkstemp(suffix=".jsonl")
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            for line in lines:
                fh.write(json.dumps(line) + "\n")
        self.addCleanup(os.remove, path)
        return path

    def test_rejects_nonpositive_box(self):
        path = self._write_jsonl([
            {"box": [0.0, 1.0, 1.0],
             "particles": [{"id": 1, "type": "A", "position": [0, 0, 0]}]},
        ])
        with self.assertRaises(ConfigError):
            load_trajectory(path)

    def test_rejects_inconsistent_particles(self):
        path = self._write_jsonl([
            {"box": [1, 1, 1],
             "particles": [{"id": 1, "type": "A", "position": [0, 0, 0]}]},
            {"box": [1, 1, 1],
             "particles": [{"id": 2, "type": "A", "position": [0, 0, 0]}]},
        ])
        with self.assertRaises(ConfigError):
            load_trajectory(path)

    def test_accepts_positions_outside_box(self):
        path = self._write_jsonl([
            {"box": [10, 10, 10],
             "particles": [{"id": 1, "type": "A", "position": [12.5, -3, 0]}]},
        ])
        frames = load_trajectory(path)
        self.assertEqual(frames[0].positions[0], (12.5, -3.0, 0.0))


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            traj = os.path.join(tmp, "traj.jsonl")
            with open(traj, "w", encoding="utf-8") as fh:
                for scale in (10.0, 12.0):
                    fh.write(json.dumps({
                        "box": [scale, scale, scale],
                        "particles": [
                            {"id": 1, "type": "A",
                             "position": [0.1, 5, 5]},
                            {"id": 2, "type": "B",
                             "position": [scale - 0.1, 5, 5]},
                        ],
                    }) + "\n")
            csv_path = os.path.join(tmp, "out.csv")
            json_path = os.path.join(tmp, "summary.json")
            cfg = os.path.join(tmp, "cfg.json")
            with open(cfg, "w", encoding="utf-8") as fh:
                json.dump({
                    "trajectory": traj,
                    "pairs": [["A", "B"], {"a": "A", "b": "A"}],
                    "r_max": 4.0,
                    "n_bins": 8,
                    "output_csv": csv_path,
                    "summary_json": json_path,
                }, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "pair_distribution", cfg],
                capture_output=True, text=True,
                cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(csv_path, encoding="utf-8") as fh:
                rows = fh.read().strip().splitlines()
            # header + 2 pairs * (8 mean + 2 frames * 8)
            self.assertEqual(len(rows), 1 + 2 * 8 * 3)
            with open(json_path, encoding="utf-8") as fh:
                summary = json.load(fh)
            self.assertEqual(summary["n_frames"], 2)
            self.assertEqual(summary["r_max_limit_half_shortest_edge"], 5.0)
            pair_ab = summary["pairs"][0]
            self.assertEqual(sum(pair_ab["total_counts"]), 2)
            pair_aa = summary["pairs"][1]
            self.assertEqual(pair_aa["empty_frames"], 2)


if __name__ == "__main__":
    unittest.main()
