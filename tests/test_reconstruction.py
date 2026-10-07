"""Tests for the parallel-beam reconstruction pipeline."""

from __future__ import annotations

import math
import os
import tempfile
import unittest

from pbr.config import Geometry, load_config
from pbr.disk import disk_projection
from pbr.fbp import ramp_filter, reconstruct
from pbr.projections import ProjectionError, load_projections


def make_geometry(n_det=33, spacing=1.0, nx=33, ny=33, dx=1.0, dy=1.0):
    return Geometry(n_det, spacing, nx, ny, dx, dy)


def disk_sinogram(geometry, n_angles, radius, attenuation, center=(0.0, 0.0)):
    positions = geometry.detector_positions()
    step = math.pi / n_angles
    return [
        (
            i * step,
            disk_projection(i * step, positions, radius, attenuation, center),
        )
        for i in range(n_angles)
    ]


class RampFilterTest(unittest.TestCase):
    def test_impulse_response_scale(self):
        # Delta at the centre: output at that tap is (1/L) * sum_k |f_k|
        # with f_k in cycles per unit length, L = 32 (padding of 16 -> 32).
        n = 16
        out = ramp_filter([0.0] * 8 + [1.0] + [0.0] * 7, 1.0)
        padded = 32
        bins = [k if k <= padded // 2 else k - padded for k in range(padded)]
        expected = sum(abs(k) / padded for k in bins) / padded
        self.assertAlmostEqual(out[8], expected, places=12)
        self.assertAlmostEqual(out[8], 0.25, places=12)

    def test_no_max_normalization(self):
        out = ramp_filter([0.0] * 8 + [1.0] + [0.0] * 7, 1.0)
        self.assertLess(max(out), 1.0)
        self.assertTrue(any(v < 0.0 for v in out))


class DiskReconstructionTest(unittest.TestCase):
    def test_centered_disk_amplitude_and_direction(self):
        geometry = make_geometry()
        attenuation = 2.5
        projections = disk_sinogram(geometry, 60, radius=10.0, attenuation=attenuation)
        image = reconstruct(projections, geometry)
        center = image[16][16]
        # FBP of a disk should recover the attenuation value itself.
        self.assertAlmostEqual(center, attenuation, delta=0.15 * attenuation)
        # Interior beats exterior (direction of contrast is preserved).
        corner = image[2][2]
        self.assertGreater(center, corner + 1.0)
        # Rotational symmetry: opposite pixels agree closely.
        self.assertAlmostEqual(image[10][16], image[22][16], delta=0.05)
        self.assertAlmostEqual(image[16][10], image[16][22], delta=0.05)

    def test_offcenter_disk_location(self):
        geometry = make_geometry()
        projections = disk_sinogram(
            geometry, 60, radius=5.0, attenuation=1.0, center=(6.0, 0.0)
        )
        image = reconstruct(projections, geometry)
        # Disk centre (x=+6 -> column 22, row 16) recovers the attenuation.
        self.assertAlmostEqual(image[16][22], 1.0, delta=0.1)
        # Mirrored location (x=-6) stays near zero: direction is correct.
        self.assertLess(abs(image[16][10]), 0.1)
        # Centre of mass of the positive part sits at the disk centre.
        total = 0.0
        weighted = 0.0
        for iy, row in enumerate(image):
            for ix, v in enumerate(row):
                if v > 0.5:
                    total += v
                    weighted += v * (ix - 16)
        self.assertAlmostEqual(weighted / total, 6.0, delta=0.5)

    def test_negative_values_preserved(self):
        geometry = make_geometry()
        projections = disk_sinogram(geometry, 60, radius=8.0, attenuation=1.0)
        image = reconstruct(projections, geometry)
        flat = [v for row in image for v in row]
        self.assertTrue(any(v < 0.0 for v in flat))


class ProjectionValidationTest(unittest.TestCase):
    def _write(self, text):
        fd, path = tempfile.mkstemp(suffix=".csv")
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(text)
        self.addCleanup(os.remove, path)
        return path

    def test_missing_angle_rejected(self):
        path = self._write("0,1,2\n45,1,2\n135,1,2\n")
        with self.assertRaises(ProjectionError):
            load_projections(path, 2)

    def test_duplicate_angle_rejected(self):
        path = self._write("0,1,2\n0,3,4\n90,1,2\n135,1,2\n")
        with self.assertRaises(ProjectionError):
            load_projections(path, 2)

    def test_endpoint_180_rejected(self):
        path = self._write("0,1,2\n60,1,2\n120,1,2\n180,1,2\n")
        with self.assertRaises(ProjectionError):
            load_projections(path, 2)

    def test_wrong_detector_count_rejected(self):
        path = self._write("0,1,2,3\n90,1,2,3\n")
        with self.assertRaises(ProjectionError):
            load_projections(path, 2)

    def test_valid_uniform_set_accepted(self):
        path = self._write("90,1,2\n0,3,4\n45,5,6\n135,7,8\n")
        projections = load_projections(path, 2)
        self.assertEqual(len(projections), 4)
        self.assertAlmostEqual(projections[0][0], 0.0)
        self.assertAlmostEqual(projections[-1][0], math.radians(135.0))


class GeometryTest(unittest.TestCase):
    def test_even_grid_not_half_pixel_shifted(self):
        geometry = make_geometry(n_det=4, nx=4, ny=4)
        positions = geometry.detector_positions()
        self.assertEqual(positions, [-1.5, -0.5, 0.5, 1.5])
        x_min, x_max = geometry.x_range()
        self.assertEqual((x_min, x_max), (-1.5, 1.5))


if __name__ == "__main__":
    unittest.main()
