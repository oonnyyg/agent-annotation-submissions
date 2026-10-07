"""Tests for the parallel-beam FBP reconstruction tool."""

from __future__ import annotations

import json
import math
import tempfile
import unittest
from pathlib import Path

import fbp
import main


def write_csv(path, rows):
    text = "\n".join(",".join(str(v) for v in row) for row in rows)
    Path(path).write_text(text + "\n", encoding="utf-8")


def make_config(tmp, nx=32, ny=32, pixel_size=0.25, spacing=0.5):
    config = {
        "detector_spacing": spacing,
        "image": {"nx": nx, "ny": ny, "pixel_size": pixel_size},
    }
    path = Path(tmp) / "config.json"
    path.write_text(json.dumps(config), encoding="utf-8")
    return str(path)


class ValidationTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = self.tmp.name

    def load(self, rows):
        path = Path(self.dir) / "proj.csv"
        write_csv(path, rows)
        return fbp.load_sinogram(str(path))

    def test_missing_angle_rejected(self):
        rows = [[0.0, 1.0], [90.0, 1.0], [180.0 - 1e-12, 1.0]]
        # three rows imply a 60-degree grid; these angles do not fit
        with self.assertRaises(fbp.ReconstructionError):
            self.load(rows)

    def test_duplicate_angle_rejected(self):
        rows = [[0.0, 1.0], [0.0, 2.0], [90.0, 1.0], [135.0, 1.0]]
        with self.assertRaisesRegex(fbp.ReconstructionError, "duplicate"):
            self.load(rows)

    def test_endpoint_180_rejected(self):
        rows = [[0.0, 1.0], [60.0, 1.0], [120.0, 1.0], [180.0, 1.0]]
        with self.assertRaises(fbp.ReconstructionError):
            self.load(rows)

    def test_unsorted_rows_sorted(self):
        angles, rows = self.load(
            [[90.0, 2.0], [0.0, 1.0], [135.0, 4.0], [45.0, 3.0]]
        )
        self.assertEqual(angles, [0.0, 45.0, 90.0, 135.0])
        self.assertEqual([r[0] for r in rows], [1.0, 3.0, 2.0, 4.0])

    def test_inconsistent_detector_count_rejected(self):
        with self.assertRaises(fbp.ReconstructionError):
            self.load([[0.0, 1.0, 2.0], [90.0, 1.0]])

    def test_too_many_angles_rejected(self):
        rows = [[k * 180.0 / 65.0, 1.0] for k in range(65)]
        with self.assertRaisesRegex(fbp.ReconstructionError, "angles"):
            self.load(rows)

    def test_too_many_detectors_rejected(self):
        rows = [[0.0] + [1.0] * 65, [90.0] + [1.0] * 65]
        with self.assertRaisesRegex(fbp.ReconstructionError, "detectors"):
            self.load(rows)

    def test_oversized_image_rejected(self):
        config = {
            "detector_spacing": 1.0,
            "image": {"nx": 65, "ny": 8, "pixel_size": 1.0},
        }
        path = Path(self.dir) / "bad.json"
        path.write_text(json.dumps(config), encoding="utf-8")
        with self.assertRaisesRegex(fbp.ReconstructionError, "64"):
            fbp.load_config(str(path))


class GeometryTests(unittest.TestCase):
    def test_even_grid_not_half_shifted(self):
        coords = fbp.centered_coordinates(4, 1.0)
        self.assertEqual(coords, [-1.5, -0.5, 0.5, 1.5])

    def test_odd_grid_has_zero_center(self):
        coords = fbp.centered_coordinates(5, 2.0)
        self.assertEqual(coords, [-4.0, -2.0, 0.0, 2.0, 4.0])


class RampFilterTests(unittest.TestCase):
    def test_delta_response_matches_analytic_ramp_kernel(self):
        # Sampled ramp kernel: a unit-height delta sample represents an
        # integral of a, so the response is a * h(n a) with the
        # continuous kernel h(0) = 1/(4 a^2),
        # h(n a) = -1/(pi^2 n^2 a^2) for odd n, 0 for even n != 0.
        n = 16
        spacing = 0.5
        delta = [0.0] * n
        delta[n // 2] = 1.0
        filtered = fbp.ramp_filter_projection(delta, spacing)
        center = n // 2
        self.assertAlmostEqual(
            filtered[center], 1.0 / (4.0 * spacing), places=6
        )
        for offset in (1, 2, 3, 4, 5):
            if offset % 2 == 1:
                expected = -1.0 / (math.pi**2 * offset**2 * spacing)
            else:
                expected = 0.0
            # finite (band-limited) ramp deviates slightly from the
            # continuous kernel away from the center tap
            self.assertAlmostEqual(
                filtered[center + offset], expected, delta=2e-3
            )
            self.assertAlmostEqual(
                filtered[center - offset], expected, delta=2e-3
            )

    def test_filter_is_linear(self):
        base = [float((i * 7) % 5) for i in range(16)]
        once = fbp.ramp_filter_projection(base, 1.0)
        twice = fbp.ramp_filter_projection([2.0 * v for v in base], 1.0)
        for a, b in zip(once, twice):
            self.assertAlmostEqual(2.0 * a, b, places=9)

    def test_no_max_normalization(self):
        filtered = fbp.ramp_filter_projection([1.0] + [0.0] * 15, 2.0)
        self.assertAlmostEqual(filtered[0], 0.125, places=6)


class ReconstructionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = self.tmp.name
        self.n_angles = 60
        self.n_det = 32
        self.spacing = 0.5
        self.angles = [k * 180.0 / self.n_angles for k in range(self.n_angles)]
        self.config = {
            "detector_spacing": self.spacing,
            "nx": 32,
            "ny": 32,
            "pixel_size": 0.25,
        }

    def run_disk(self, radius=2.0, amplitude=1.0):
        sinogram = fbp.disk_sinogram(
            self.angles, self.n_det, self.spacing, radius, amplitude
        )
        return fbp.reconstruct(self.angles, sinogram, self.config)

    def test_disk_center_amplitude_and_direction(self):
        image, summary = self.run_disk(radius=2.0, amplitude=1.0)
        center = image[16][16]
        # Ramp FBP of the analytic disk sinogram recovers the amplitude
        # scale (positive, order unity) at the disk center.
        self.assertGreater(center, 0.5)
        self.assertLess(center, 2.0)
        self.assertGreater(center, 0.0)
        self.assertEqual(summary["filter"]["name"], "ramp")

    def test_disk_center_symmetry(self):
        image, _ = self.run_disk()
        n = len(image)
        for iy in range(n):
            for ix in range(n):
                self.assertAlmostEqual(
                    image[iy][ix], image[iy][n - 1 - ix], places=8
                )
                self.assertAlmostEqual(
                    image[iy][ix], image[n - 1 - iy][ix], places=8
                )

    def test_negative_values_preserved(self):
        # The ramp filter produces negative sidelobes outside the disk;
        # reconstruction must keep them instead of clipping to zero.
        image, summary = self.run_disk()
        self.assertLess(summary["statistics"]["min"], 0.0)
        flat = [v for row in image for v in row]
        self.assertTrue(any(v < 0.0 for v in flat))

    def test_summary_ranges(self):
        _, summary = self.run_disk()
        self.assertEqual(summary["image"]["x_range"], [-3.875, 3.875])
        self.assertEqual(summary["detector"]["coordinate_range"], [-7.75, 7.75])
        self.assertEqual(summary["angles"]["step_degrees"], 3.0)
        self.assertFalse(summary["angles"]["right_endpoint_included"])
        self.assertGreaterEqual(summary["filter"]["zero_padding_factor"], 2.0)


class CliTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.dir = self.tmp.name

    def test_end_to_end_disk(self):
        n_angles, n_det, spacing = 36, 32, 0.5
        angles = [k * 180.0 / n_angles for k in range(n_angles)]
        sinogram = fbp.disk_sinogram(angles, n_det, spacing, radius=2.0)
        proj = Path(self.dir) / "proj.csv"
        write_csv(proj, [[a] + row for a, row in zip(angles, sinogram)])
        config = make_config(self.dir)
        out = Path(self.dir) / "out.csv"
        summ = Path(self.dir) / "summary.json"
        code = main.main(
            [
                "--projections", str(proj),
                "--config", config,
                "--output", str(out),
                "--summary", str(summ),
            ]
        )
        self.assertEqual(code, 0)
        rows = out.read_text(encoding="utf-8").strip().splitlines()
        self.assertEqual(len(rows), 32)
        self.assertEqual(len(rows[0].split(",")), 32)
        summary = json.loads(summ.read_text(encoding="utf-8"))
        self.assertEqual(summary["filter"]["name"], "ramp")

    def test_cli_reports_invalid_input(self):
        proj = Path(self.dir) / "proj.csv"
        write_csv(proj, [[0.0, 1.0], [0.0, 2.0]])
        config = make_config(self.dir)
        code = main.main(
            [
                "--projections", str(proj),
                "--config", config,
                "--output", str(Path(self.dir) / "o.csv"),
                "--summary", str(Path(self.dir) / "s.json"),
            ]
        )
        self.assertEqual(code, 1)


if __name__ == "__main__":
    unittest.main()
