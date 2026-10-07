import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from quality_unwrap import (
    TWO_PI,
    diagnostics_dict,
    normalize_phase,
    unwrap,
)


def make_grid(rows, cols, value):
    return [[value] * cols for _ in range(rows)]


class NormalizeTests(unittest.TestCase):
    def test_half_open_interval(self):
        self.assertEqual(normalize_phase(math.pi), -math.pi)
        self.assertEqual(normalize_phase(-math.pi), -math.pi)
        self.assertAlmostEqual(normalize_phase(3 * math.pi), -math.pi)
        self.assertAlmostEqual(normalize_phase(0.5), 0.5)
        self.assertAlmostEqual(normalize_phase(-0.5), -0.5)
        self.assertAlmostEqual(normalize_phase(4 * math.pi + 1.0), 1.0)


class RampTests(unittest.TestCase):
    def test_cross_cycle_ramp(self):
        # True unwrapped phase is a ramp stepping by 1.0 rad per column;
        # the wrapped input crosses the pi boundary several times.
        cols = 8
        true_phase = [[1.0 * c for c in range(cols)]]
        wrapped = [[normalize_phase(v) for v in true_phase[0]]]
        quality = make_grid(1, cols, 1.0)
        result = unwrap(wrapped, quality)
        self.assertEqual(len(result.roots), 1)
        self.assertEqual(result.conflicts, [])
        for c in range(cols):
            self.assertAlmostEqual(result.unwrapped[c], true_phase[0][c])
            self.assertEqual(result.cycle_offsets[c], round((true_phase[0][c] - wrapped[0][c]) / TWO_PI))

    def test_root_is_highest_quality(self):
        wrapped = [[0.1, 0.2, 0.3]]
        quality = [[0.5, 0.9, 0.7]]
        result = unwrap(wrapped, quality)
        self.assertEqual(result.roots, [1])
        self.assertAlmostEqual(result.unwrapped[1], 0.2)
        self.assertAlmostEqual(result.unwrapped[0], 0.1)
        self.assertAlmostEqual(result.unwrapped[2], 0.3)

    def test_root_tie_breaks_row_major(self):
        wrapped = [[0.1, 0.2], [0.3, 0.4]]
        quality = make_grid(2, 2, 1.0)
        result = unwrap(wrapped, quality)
        self.assertEqual(result.roots, [0])


class MaskTests(unittest.TestCase):
    def test_mask_splits_components(self):
        wrapped = [[0.1, 0.0, 0.2]]
        quality = make_grid(1, 3, 1.0)
        mask = [[0.0, 1.0, 0.0]]
        result = unwrap(wrapped, quality, mask)
        self.assertEqual(len(result.roots), 2)
        self.assertEqual(result.labels[0], 0)
        self.assertEqual(result.labels[1], -1)
        self.assertEqual(result.labels[2], 1)
        self.assertIsNone(result.unwrapped[1])
        self.assertIsNone(result.cycle_offsets[1])

    def test_isolated_pixel_forms_own_region(self):
        wrapped = [[0.7]]
        quality = [[2.0]]
        mask = [[0.0]]
        result = unwrap(wrapped, quality, mask)
        self.assertEqual(len(result.roots), 1)
        self.assertAlmostEqual(result.unwrapped[0], 0.7)
        self.assertEqual(result.cycle_offsets[0], 0)

    def test_masked_phase_may_be_nonfinite(self):
        wrapped = [[float("nan"), 0.5]]
        quality = [[0.0, 1.0]]
        mask = [[1.0, 0.0]]
        result = unwrap(wrapped, quality, mask)
        self.assertEqual(len(result.roots), 1)


class ConflictTests(unittest.TestCase):
    def test_inconsistent_loop_is_reported_not_hidden(self):
        # Residue loop: walking the 4-cycle accumulates exactly one full
        # cycle (each edge contributes +pi/2), so no spanning tree can
        # satisfy every edge.
        half_pi = math.pi / 2
        wrapped = [
            [0.0, half_pi],
            [-half_pi, -math.pi],
        ]
        quality = make_grid(2, 2, 1.0)
        # Lower the quality of one edge so the tree drops exactly that edge.
        quality[1][1] = 0.1
        result = unwrap(wrapped, quality)
        self.assertEqual(len(result.roots), 1)
        self.assertEqual(len(result.conflicts), 1)
        conflict = result.conflicts[0]
        self.assertEqual(conflict["cycle_difference"], 1)
        self.assertEqual(conflict["from"], [1, 0])
        self.assertEqual(conflict["to"], [1, 1])
        # The result is still fully populated despite the conflict.
        for value in result.unwrapped:
            self.assertIsNotNone(value)

    def test_consistent_grid_has_no_conflicts(self):
        wrapped = [[0.1, 0.2], [0.3, 0.4]]
        quality = make_grid(2, 2, 1.0)
        result = unwrap(wrapped, quality)
        self.assertEqual(result.conflicts, [])


class ValidationTests(unittest.TestCase):
    def test_nonfinite_phase_rejected(self):
        with self.assertRaises(ValueError):
            unwrap([[float("inf")]], [[1.0]])

    def test_negative_quality_rejected(self):
        with self.assertRaises(ValueError):
            unwrap([[0.0]], [[-0.5]])

    def test_shape_mismatch_rejected(self):
        with self.assertRaises(ValueError):
            unwrap([[0.0, 0.1]], [[1.0]])


class CliTests(unittest.TestCase):
    def test_cli_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            phase_path = os.path.join(tmp, "phase.csv")
            quality_path = os.path.join(tmp, "quality.csv")
            mask_path = os.path.join(tmp, "mask.csv")
            out_u = os.path.join(tmp, "unwrapped.csv")
            out_l = os.path.join(tmp, "labels.csv")
            out_d = os.path.join(tmp, "diag.json")
            with open(phase_path, "w", encoding="utf-8") as fh:
                fh.write("0.0,1.5707963267948966\n-1.5707963267948966,-3.141592653589793\n")
            with open(quality_path, "w", encoding="utf-8") as fh:
                fh.write("1.0,1.0\n1.0,0.1\n")
            with open(mask_path, "w", encoding="utf-8") as fh:
                fh.write("0,0\n0,0\n")
            subprocess.run(
                [
                    sys.executable,
                    "quality_unwrap.py",
                    phase_path,
                    quality_path,
                    "--mask",
                    mask_path,
                    "--out-unwrapped",
                    out_u,
                    "--out-labels",
                    out_l,
                    "--diagnostics",
                    out_d,
                ],
                check=True,
                cwd=os.path.dirname(os.path.abspath(__file__)),
            )
            with open(out_d, encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertEqual(diag["component_count"], 1)
            self.assertEqual(diag["conflict_count"], 1)
            with open(out_l, encoding="utf-8") as fh:
                self.assertEqual(fh.read().split(), ["0,0", "0,0"])


if __name__ == "__main__":
    unittest.main()
