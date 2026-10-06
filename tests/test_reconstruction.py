import math
import unittest

from stereo_ray.geometry import (
    Camera,
    ReconstructionError,
    project_point,
    reconstruct_point,
    undistort_normalized,
)

IDENTITY = ((1.0, 0.0, 0.0), (0.0, 1.0, 0.0), (0.0, 0.0, 1.0))


def make_camera(center, k1=0.0, k2=0.0, rotation=IDENTITY):
    return Camera(
        fx=800.0, fy=820.0, cx=320.0, cy=240.0,
        center=center, rotation=rotation, k1=k1, k2=k2,
    )


def distort_pixel(cam, point):
    """Forward projection through the full model (ground-truth observation)."""
    u, v, _ = project_point(cam, point)
    return u, v


class UndistortTests(unittest.TestCase):
    def test_roundtrip(self):
        k1, k2 = 0.15, 0.02
        for xd, yd in [(0.0, 0.0), (0.3, -0.2), (-0.5, 0.4)]:
            xn, yn = undistort_normalized(xd, yd, k1, k2)
            r2 = xn * xn + yn * yn
            mag = 1.0 + k1 * r2 + k2 * r2 * r2
            self.assertAlmostEqual(xn * mag, xd, places=12)
            self.assertAlmostEqual(yn * mag, yd, places=12)


class ReconstructionTests(unittest.TestCase):
    def test_exact_point_no_distortion(self):
        cam1 = make_camera((0.0, 0.0, 0.0))
        cam2 = make_camera((1.0, 0.0, 0.0))
        target = (0.3, -0.2, 5.0)
        px1 = distort_pixel(cam1, target)
        px2 = distort_pixel(cam2, target)
        result = reconstruct_point(cam1, cam2, px1, px2)
        for got, want in zip(result["point"], target):
            self.assertAlmostEqual(got, want, places=9)
        self.assertAlmostEqual(result["ray_distance"], 0.0, places=9)
        for pair in result["residuals_px"]:
            self.assertAlmostEqual(pair[0], 0.0, places=6)
            self.assertAlmostEqual(pair[1], 0.0, places=6)

    def test_distorted_observations(self):
        cam1 = make_camera((0.0, 0.0, 0.0), k1=0.2, k2=0.05)
        cam2 = make_camera((1.0, 0.1, 0.0), k1=0.1, k2=0.01)
        target = (-0.4, 0.3, 4.0)
        px1 = distort_pixel(cam1, target)
        px2 = distort_pixel(cam2, target)
        result = reconstruct_point(cam1, cam2, px1, px2)
        for got, want in zip(result["point"], target):
            self.assertAlmostEqual(got, want, places=7)
        self.assertLess(result["ray_distance"], 1e-6)

    def test_skew_rays_midpoint_and_distance(self):
        cam1 = make_camera((0.0, 0.0, 0.0))
        cam2 = make_camera((1.0, 0.0, 0.0))
        # Same x angles, but right image y shifted by 8 px -> skew rays.
        px1 = (400.0, 240.0)
        px2 = (336.0, 248.0)
        result = reconstruct_point(cam1, cam2, px1, px2)
        self.assertGreater(result["ray_distance"], 0.0)
        # Midpoint must reproject close to both observations.
        for pair in result["residuals_px"]:
            self.assertLess(math.hypot(*pair), 8.0)

    def test_behind_camera_rejected(self):
        cam1 = make_camera((0.0, 0.0, 0.0))
        cam2 = make_camera((1.0, 0.0, 0.0))
        # Rays diverge: intersection of the lines lies behind both cameras.
        px1 = (240.0, 240.0)   # looks left
        px2 = (400.0, 240.0)   # looks right
        with self.assertRaises(ReconstructionError):
            reconstruct_point(cam1, cam2, px1, px2)

    def test_parallel_rays_rejected(self):
        cam1 = make_camera((0.0, 0.0, 0.0))
        cam2 = make_camera((1.0, 0.0, 0.0))
        with self.assertRaises(ReconstructionError):
            reconstruct_point(cam1, cam2, (400.0, 240.0), (400.0, 240.0))

    def test_coincident_centres_rejected(self):
        cam1 = make_camera((0.0, 0.0, 0.0))
        cam2 = make_camera((0.0, 0.0, 0.0))
        with self.assertRaises(ReconstructionError):
            reconstruct_point(cam1, cam2, (320.0, 240.0), (330.0, 240.0))

    def test_angle_threshold_configurable(self):
        cam1 = make_camera((0.0, 0.0, 0.0))
        cam2 = make_camera((0.05, 0.0, 0.0))  # small baseline
        px1 = (400.0, 240.0)
        px2 = (320.0, 240.0)
        # Narrow angle passes with a tiny threshold, fails with a large one.
        reconstruct_point(cam1, cam2, px1, px2, min_angle_deg=0.01)
        with self.assertRaises(ReconstructionError):
            reconstruct_point(cam1, cam2, px1, px2, min_angle_deg=45.0)
        with self.assertRaises(ValueError):
            reconstruct_point(cam1, cam2, px1, px2, min_angle_deg=0.0)
        with self.assertRaises(ValueError):
            reconstruct_point(cam1, cam2, px1, px2, min_angle_deg=90.0)


if __name__ == "__main__":
    unittest.main()
