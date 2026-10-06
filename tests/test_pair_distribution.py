import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from pair_distribution.config import PairSpec, load_config
from pair_distribution.errors import PairDistributionError
from pair_distribution.rdf import check_r_max, compute_pair
from pair_distribution.trajectory import Frame, load_trajectory


def make_frame(index, box, particles):
    return Frame(index=index, box=tuple(box), particles={
        pid: (ptype, tuple(pos)) for pid, ptype, pos in particles
    })


class TrajectoryTests(unittest.TestCase):
    def write_jsonl(self, directory, rows):
        path = os.path.join(directory, "traj.jsonl")
        with open(path, "w", encoding="utf-8") as handle:
            for row in rows:
                handle.write(json.dumps(row) + "\n")
        return path

    def test_missing_particle_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_jsonl(tmp, [
                {"box": [10, 10, 10], "particles": [
                    {"id": 1, "type": "A", "position": [0, 0, 0]},
                    {"id": 2, "type": "A", "position": [1, 0, 0]},
                ]},
                {"box": [10, 10, 10], "particles": [
                    {"id": 1, "type": "A", "position": [0, 0, 0]},
                ]},
            ])
            with self.assertRaises(PairDistributionError):
                load_trajectory(path)

    def test_nonpositive_box_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_jsonl(tmp, [
                {"box": [10, -5, 10], "particles": [
                    {"id": 1, "type": "A", "position": [0, 0, 0]},
                ]},
            ])
            with self.assertRaises(PairDistributionError):
                load_trajectory(path)

    def test_species_change_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = self.write_jsonl(tmp, [
                {"box": [10, 10, 10], "particles": [
                    {"id": 1, "type": "A", "position": [0, 0, 0]},
                ]},
                {"box": [10, 10, 10], "particles": [
                    {"id": 1, "type": "B", "position": [0, 0, 0]},
                ]},
            ])
            with self.assertRaises(PairDistributionError):
                load_trajectory(path)


class RdfTests(unittest.TestCase):
    def test_min_image_across_boundary(self):
        frame = make_frame(0, [10, 10, 10], [
            (1, "A", [0.5, 5.0, 5.0]),
            (2, "A", [9.6, 5.0, 5.0]),
        ])
        result = compute_pair([frame], PairSpec("A", "A"), 10, 0.5, 5.0)
        # distance 0.9 -> bin 1 ([0.5, 1.0)); ordered pair counts 2
        self.assertEqual(result.histograms[0].counts[1], 2)
        self.assertEqual(sum(result.histograms[0].counts), 2)

    def test_same_species_double_count_and_cn(self):
        frame = make_frame(0, [10, 10, 10], [
            (1, "A", [1.0, 5.0, 5.0]),
            (2, "A", [2.0, 5.0, 5.0]),
        ])
        result = compute_pair([frame], PairSpec("A", "A"), 10, 0.5, 5.0)
        self.assertEqual(result.histograms[0].counts[2], 2)  # r=1.0 -> bin 2
        self.assertAlmostEqual(result.cn_avg[9], 1.0)  # one neighbour per centre

    def test_histogram_edges(self):
        # r exactly on a boundary falls into the right-hand bin; r == r_max excluded
        frame = make_frame(0, [10, 10, 10], [
            (1, "A", [5.0, 5.0, 5.0]),
            (2, "A", [6.0, 5.0, 5.0]),   # r = 1.0 exactly -> bin 2, not bin 1
            (3, "A", [0.0, 5.0, 5.0]),   # r(1,3) = 5.0 == r_max -> excluded
        ])
        result = compute_pair([frame], PairSpec("A", "A"), 10, 0.5, 5.0)
        counts = result.histograms[0].counts
        self.assertEqual(counts[1], 0)
        self.assertEqual(counts[2], 2)   # 1<->2 ordered
        self.assertEqual(counts[8], 2)   # 2<->3 min image r = 4.0 -> bin 8
        self.assertEqual(sum(counts), 4)

    def test_r_max_excluded(self):
        frame = make_frame(0, [10, 10, 10], [
            (1, "A", [0.0, 5.0, 5.0]),
            (2, "A", [5.0, 5.0, 5.0]),  # r = 5.0 == r_max
        ])
        result = compute_pair([frame], PairSpec("A", "A"), 10, 0.5, 5.0)
        self.assertEqual(sum(result.histograms[0].counts), 0)

    def test_r_max_limit(self):
        frames = [
            make_frame(0, [10, 10, 10], [(1, "A", [0, 0, 0])]),
            make_frame(1, [6, 8, 8], [(1, "A", [0, 0, 0])]),
        ]
        self.assertAlmostEqual(check_r_max(frames, 3.0), 3.0)
        with self.assertRaises(PairDistributionError):
            check_r_max(frames, 3.5)

    def test_per_frame_normalization_with_changing_box(self):
        # Box volume changes between frames and the pair distance falls in
        # different bins, so per-frame normalization is distinguishable from
        # pooling counts with a single average volume.
        frames = [
            make_frame(0, [10, 10, 10], [
                (1, "A", [5.0, 5.0, 5.0]),
                (2, "B", [6.0, 5.0, 5.0]),
            ]),
            make_frame(1, [12, 12, 12], [
                (1, "A", [6.0, 6.0, 6.0]),
                (2, "B", [7.5, 6.0, 6.0]),
            ]),
        ]
        n_bins, r_max = 10, 5.0
        dr = 0.5
        result = compute_pair(frames, PairSpec("A", "B"), n_bins, dr, r_max)
        shell2 = 4.0 / 3.0 * math.pi * (1.5 ** 3 - 1.0 ** 3)
        shell3 = 4.0 / 3.0 * math.pi * (2.0 ** 3 - 1.5 ** 3)
        self.assertAlmostEqual(result.g_avg[2], (1.0 / ((1 / 1000.0) * shell2)) / 2.0)
        self.assertAlmostEqual(result.g_avg[3], (1.0 / ((1 / 1728.0) * shell3)) / 2.0)
        # pooling counts with the mean volume would give a different value
        mean_volume = (1000.0 + 1728.0) / 2.0
        pooled = 1.0 / ((1 / mean_volume) * shell2)
        self.assertNotAlmostEqual(result.g_avg[2], pooled, places=6)

    def test_frame_order_invariance(self):
        frames = [
            make_frame(0, [10, 10, 10], [
                (1, "A", [1.0, 5.0, 5.0]),
                (2, "A", [2.3, 5.0, 5.0]),
                (3, "B", [1.7, 5.0, 5.0]),
            ]),
            make_frame(1, [11, 10, 9], [
                (1, "A", [1.1, 5.0, 5.0]),
                (2, "A", [2.9, 5.0, 5.0]),
                (3, "B", [1.4, 5.0, 5.0]),
            ]),
            make_frame(2, [9, 10, 11], [
                (1, "A", [1.2, 5.0, 5.0]),
                (2, "A", [2.1, 5.0, 5.0]),
                (3, "B", [2.0, 5.0, 5.0]),
            ]),
        ]
        pair = PairSpec("A", "B")
        forward = compute_pair(frames, pair, 10, 0.5, 4.5)
        backward = compute_pair(list(reversed(frames)), pair, 10, 0.5, 4.5)
        self.assertEqual(forward.g_avg, backward.g_avg)
        self.assertEqual(forward.cn_avg, backward.cn_avg)

    def test_missing_species_gives_empty(self):
        frame = make_frame(0, [10, 10, 10], [(1, "A", [0, 0, 0])])
        result = compute_pair([frame], PairSpec("A", "C"), 10, 0.5, 5.0)
        self.assertTrue(result.empty)
        self.assertTrue(all(v == 0.0 for v in result.cn_avg))

    def test_single_same_species_gives_empty(self):
        frame = make_frame(0, [10, 10, 10], [(1, "A", [0, 0, 0])])
        result = compute_pair([frame], PairSpec("A", "A"), 10, 0.5, 5.0)
        self.assertTrue(result.empty)
        self.assertTrue(all(v == 0.0 for v in result.cn_avg))

    def test_coordinates_outside_box(self):
        frame = make_frame(0, [10, 10, 10], [
            (1, "A", [-0.4, 5.0, 5.0]),
            (2, "A", [10.3, 5.0, 5.0]),  # min-image distance 0.7
        ])
        result = compute_pair([frame], PairSpec("A", "A"), 10, 0.5, 5.0)
        self.assertEqual(result.histograms[0].counts[1], 2)


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            traj = os.path.join(tmp, "traj.jsonl")
            with open(traj, "w", encoding="utf-8") as handle:
                for box in ([10, 10, 10], [12, 12, 12]):
                    handle.write(json.dumps({
                        "box": box,
                        "particles": [
                            {"id": 1, "type": "A", "position": [1.0, 5.0, 5.0]},
                            {"id": 2, "type": "A", "position": [2.0, 5.0, 5.0]},
                            {"id": 3, "type": "B", "position": [1.5, 5.0, 5.0]},
                        ],
                    }) + "\n")
            config = os.path.join(tmp, "config.json")
            csv_path = os.path.join(tmp, "out.csv")
            summary_path = os.path.join(tmp, "out.json")
            with open(config, "w", encoding="utf-8") as handle:
                json.dump({
                    "trajectory": traj,
                    "pairs": [{"a": "A", "b": "A"}, {"a": "A", "b": "B"}],
                    "rdf": {"r_max": 4.0, "n_bins": 8},
                    "output_csv": csv_path,
                    "summary_json": summary_path,
                }, handle)
            proc = subprocess.run(
                [sys.executable, "-m", "pair_distribution", config],
                capture_output=True, text=True,
                cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(csv_path, encoding="utf-8") as handle:
                lines = handle.read().strip().splitlines()
            self.assertEqual(len(lines), 1 + 2 * 8)
            self.assertIn("frame_0_count", lines[0])
            with open(summary_path, encoding="utf-8") as handle:
                summary = json.load(handle)
            self.assertEqual(summary["n_frames"], 2)
            self.assertEqual(summary["n_particles"], 3)
            self.assertFalse(summary["pairs"][0]["empty"])


if __name__ == "__main__":
    unittest.main()
