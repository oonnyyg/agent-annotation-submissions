import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from phase_unwrap import GridData, load_grids, unwrap
from phase_unwrap.grid import normalize_phase

TWO_PI = 2.0 * math.pi


def make_grid(phase, quality, mask):
    rows = len(phase)
    cols = len(phase[0])
    valid = [[mask[r][c] == 0 for c in range(cols)] for r in range(rows)]
    nphase = [[normalize_phase(phase[r][c]) if valid[r][c] else math.nan
               for c in range(cols)] for r in range(rows)]
    nqual = [[float(quality[r][c]) if valid[r][c] else math.nan
              for c in range(cols)] for r in range(rows)]
    return GridData(rows=rows, cols=cols, phase=nphase,
                    quality=nqual, valid=valid)


class RampTest(unittest.TestCase):
    def test_ramp_crossing_wrap(self):
        cols = 8
        true_phase = [i * 1.5 for i in range(cols)]
        wrapped = [normalize_phase(v) for v in true_phase]
        grid = make_grid([wrapped], [[1.0] * cols], [[0] * cols])
        result = unwrap(grid)
        self.assertEqual(len(result.roots), 1)
        self.assertEqual(result.conflicts, [])
        base_k = result.cycles[0][0]
        for c in range(cols):
            self.assertAlmostEqual(
                result.unwrapped[0][c] - result.unwrapped[0][0],
                true_phase[c] - true_phase[0], places=9)
            expected_k = round((true_phase[c] - wrapped[c]) / TWO_PI)
            self.assertEqual(result.cycles[0][c] - base_k, expected_k)

    def test_root_keeps_normalized_phase(self):
        phase = [[0.5, 0.6], [0.7, 0.8]]
        quality = [[1.0, 3.0], [2.0, 0.5]]
        grid = make_grid(phase, quality, [[0, 0], [0, 0]])
        result = unwrap(grid)
        self.assertEqual(result.roots, [(0, 1)])
        self.assertAlmostEqual(result.unwrapped[0][1], 0.6)


class MaskTest(unittest.TestCase):
    def test_mask_splits_regions(self):
        phase = [[0.1, 0.2, 0.3], [0.1, 0.2, 0.3], [0.1, 0.2, 0.3]]
        quality = [[1.0] * 3 for _ in range(3)]
        mask = [[0, 1, 0], [0, 1, 0], [0, 1, 0]]
        grid = make_grid(phase, quality, mask)
        result = unwrap(grid)
        self.assertEqual(len(result.roots), 2)
        self.assertEqual(result.region[0][1], -1)
        self.assertTrue(math.isnan(result.unwrapped[1][1]))
        left = {result.region[r][0] for r in range(3)}
        right = {result.region[r][2] for r in range(3)}
        self.assertEqual(len(left), 1)
        self.assertEqual(len(right), 1)
        self.assertNotEqual(left.pop(), right.pop())

    def test_isolated_pixel_is_own_region(self):
        phase = [[0.1, 0.2, 0.3]]
        quality = [[1.0, 5.0, 1.0]]
        mask = [[0, 1, 0]]
        grid = make_grid(phase, quality, mask)
        result = unwrap(grid)
        self.assertEqual(len(result.roots), 2)
        self.assertNotEqual(result.region[0][0], result.region[0][2])


class ConflictTest(unittest.TestCase):
    def test_contradictory_loop(self):
        phase = [
            [0.0, 2.5],
            [-1.28, 2.6],
        ]
        quality = [[1.0, 1.0], [1.0, 1.0]]
        grid = make_grid(phase, quality, [[0, 0], [0, 0]])
        result = unwrap(grid)
        self.assertEqual(len(result.roots), 1)
        self.assertTrue(result.conflicts)
        total = sum(abs(c.cycles) for c in result.conflicts)
        self.assertGreaterEqual(total, 1)
        for conflict in result.conflicts:
            self.assertTrue(conflict.same_region)

    def test_consistent_grid_has_no_conflicts(self):
        phase = [[0.1 * (r + c) for c in range(4)] for r in range(4)]
        quality = [[1.0] * 4 for _ in range(4)]
        grid = make_grid(phase, quality, [[0] * 4 for _ in range(4)])
        result = unwrap(grid)
        self.assertEqual(result.conflicts, [])


class DeterminismTest(unittest.TestCase):
    def test_tie_breaking_is_deterministic(self):
        phase = [[0.2, -0.4, 0.9], [1.1, 0.0, -2.2]]
        quality = [[1.0, 1.0, 1.0], [1.0, 1.0, 1.0]]
        grid = make_grid(phase, quality, [[0] * 3, [0] * 3])
        first = unwrap(grid)
        second = unwrap(grid)
        self.assertEqual(first.unwrapped, second.unwrapped)
        self.assertEqual(first.region, second.region)
        self.assertEqual(first.roots, [(0, 0)])


class ValidationTest(unittest.TestCase):
    def _write(self, directory, name, rows):
        path = os.path.join(directory, name)
        with open(path, "w", encoding="utf-8") as handle:
            for row in rows:
                handle.write(",".join(str(v) for v in row) + "\n")
        return path

    def test_negative_quality_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            p = self._write(tmp, "p.csv", [[0.1]])
            q = self._write(tmp, "q.csv", [[-1.0]])
            m = self._write(tmp, "m.csv", [[0]])
            with self.assertRaises(ValueError):
                load_grids(p, q, m)

    def test_shape_mismatch_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            p = self._write(tmp, "p.csv", [[0.1, 0.2]])
            q = self._write(tmp, "q.csv", [[1.0]])
            m = self._write(tmp, "m.csv", [[0, 0]])
            with self.assertRaises(ValueError):
                load_grids(p, q, m)

    def test_phase_normalized_on_load(self):
        with tempfile.TemporaryDirectory() as tmp:
            p = self._write(tmp, "p.csv", [[3.5, -3.5, 7.0]])
            q = self._write(tmp, "q.csv", [[1.0, 1.0, 1.0]])
            m = self._write(tmp, "m.csv", [[0, 0, 0]])
            grid = load_grids(p, q, m)
            for value in grid.phase[0]:
                self.assertGreaterEqual(value, -math.pi)
                self.assertLess(value, math.pi)


class CliTest(unittest.TestCase):
    def test_cli_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            def write(name, rows):
                path = os.path.join(tmp, name)
                with open(path, "w", encoding="utf-8") as handle:
                    for row in rows:
                        handle.write(",".join(str(v) for v in row) + "\n")
                return path

            phase = write("p.csv", [[0.1, 0.2], [0.3, 0.4]])
            quality = write("q.csv", [[1.0, 1.0], [1.0, 1.0]])
            mask = write("m.csv", [[0, 0], [0, 0]])
            out_u = os.path.join(tmp, "u.csv")
            out_r = os.path.join(tmp, "r.csv")
            out_d = os.path.join(tmp, "d.json")
            proc = subprocess.run(
                [sys.executable, "-m", "phase_unwrap",
                 "--phase", phase, "--quality", quality, "--mask", mask,
                 "--out-unwrapped", out_u, "--out-regions", out_r,
                 "--out-diagnostics", out_d],
                capture_output=True, text=True,
                cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(out_d, encoding="utf-8") as handle:
                diag = json.load(handle)
            self.assertTrue(diag["consistent"])
            self.assertEqual(diag["num_regions"], 1)
            self.assertTrue(os.path.exists(out_u))
            self.assertTrue(os.path.exists(out_r))


if __name__ == "__main__":
    unittest.main()
