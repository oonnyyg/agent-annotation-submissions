import json
import math
import os
import tempfile
import unittest

import stereo_reconstruct as sr


def make_camera(center=(0.0, 0.0, 0.0), k1=0.0, k2=0.0, rotation=None):
    return sr.Camera(
        name="cam",
        fx=800.0,
        fy=820.0,
        cx=320.0,
        cy=240.0,
        k1=k1,
        k2=k2,
        center=center,
        rotation=rotation or ((1, 0, 0), (0, 1, 0), (0, 0, 1)),
    )


class TestCameraModel(unittest.TestCase):
    def test_undistort_roundtrip(self):
        cam = make_camera(k1=0.15, k2=0.02)
        # Distort a known undistorted normalized point, then invert.
        xu, yu = 0.3, -0.2
        r2 = xu * xu + yu * yu
        s = 1.0 + cam.k1 * r2 + cam.k2 * r2 * r2
        xd, yd = xu * s, yu * s
        rx, ry = cam.undistort_normalized(xd, yd)
        self.assertAlmostEqual(rx, xu, places=12)
        self.assertAlmostEqual(ry, yu, places=12)

    def test_project_roundtrip_with_distortion(self):
        cam = make_camera(k1=0.1, k2=0.01)
        point = (0.5, -0.3, 4.0)
        u, v, z = cam.project(point)
        self.assertAlmostEqual(z, 4.0)
        ray = cam.pixel_to_world_ray(u, v)
        # Ray must pass through the original point.
        t = point[2] / ray[2]
        self.assertAlmostEqual(ray[0] * t, point[0], places=9)
        self.assertAlmostEqual(ray[1] * t, point[1], places=9)

    def test_project_rejects_nonpositive_depth(self):
        cam = make_camera()
        with self.assertRaises(ValueError):
            cam.project((0.0, 0.0, -1.0))

    def test_invalid_rotation_rejected(self):
        with self.assertRaises(ValueError):
            sr.Camera.from_dict("c", {
                "fx": 1, "fy": 1, "cx": 0, "cy": 0,
                "center": [0, 0, 0],
                "rotation": [[1, 0, 0], [0, 2, 0], [0, 0, 1]],
            })


class TestTriangulation(unittest.TestCase):
    def setUp(self):
        self.left = make_camera(center=(0.0, 0.0, 0.0))
        self.right = make_camera(center=(1.0, 0.0, 0.0))

    def _pixel_for(self, cam, point):
        u, v, _ = cam.project(point)
        return u, v

    def test_exact_intersection(self):
        point = (0.5, 0.2, 5.0)
        px_l = self._pixel_for(self.left, point)
        px_r = self._pixel_for(self.right, point)
        rec = sr.triangulate(self.left, px_l, self.right, px_r, 1.0)
        for got, want in zip(rec["point"], point):
            self.assertAlmostEqual(got, want, places=9)
        self.assertAlmostEqual(rec["closest_distance"], 0.0, places=9)
        self.assertLess(rec["left"]["residual_norm_pixels"], 1e-6)
        self.assertLess(rec["right"]["residual_norm_pixels"], 1e-6)

    def test_skew_rays_midpoint_and_gap(self):
        # Perturb the right pixel so rays become skew.
        point = (0.5, 0.2, 5.0)
        px_l = self._pixel_for(self.left, point)
        u, v = self._pixel_for(self.right, point)
        rec = sr.triangulate(self.left, px_l, self.right, (u + 2.0, v), 1.0)
        self.assertGreater(rec["closest_distance"], 0.0)
        # Midpoint lies between the two closest points; residual should be
        # small but nonzero.
        self.assertGreater(rec["right"]["residual_norm_pixels"], 0.0)
        self.assertLess(rec["right"]["residual_norm_pixels"], 2.0)

    def test_distorted_observation(self):
        left = make_camera(center=(0, 0, 0), k1=0.2, k2=0.03)
        right = make_camera(center=(1, 0, 0), k1=0.1, k2=0.0)
        point = (0.4, -0.3, 6.0)
        px_l = self._pixel_for(left, point)
        px_r = self._pixel_for(right, point)
        rec = sr.triangulate(left, px_l, right, px_r, 1.0)
        for got, want in zip(rec["point"], point):
            self.assertAlmostEqual(got, want, places=9)

    def test_parallel_rays_rejected(self):
        # Same pixel in both cameras with identity rotations -> parallel rays.
        with self.assertRaises(sr.GeometryError):
            sr.triangulate(self.left, (400.0, 240.0), self.right, (400.0, 240.0), 1.0)

    def test_coincident_centers_rejected(self):
        other = make_camera(center=(0.0, 0.0, 0.0))
        with self.assertRaises(sr.GeometryError):
            sr.triangulate(self.left, (400.0, 240.0), other, (410.0, 240.0), 1.0)

    def test_behind_camera_rejected(self):
        # Point behind the right camera: rays' closest points have negative
        # parameter on at least one ray.
        px_l = self._pixel_for(self.left, (0.5, 0.0, 5.0))
        # Right camera pixel corresponding to a point behind it.
        right = self.right
        # Direction from right center to a point behind it (z < 0 world):
        # project manually is impossible; instead pick a pixel whose ray,
        # combined with the left ray, intersects behind the right camera.
        # Left ray toward x=0.5 at z=5; choose right pixel far right so the
        # rays diverge and the lines' closest points lie behind the cameras.
        with self.assertRaises(sr.GeometryError):
            sr.triangulate(self.left, px_l, right, (640.0, 240.0), 0.01)

    def test_antiparallel_rejected(self):
        # Cameras facing each other, rays collinear and opposite.
        rot_flip = ((-1, 0, 0), (0, 1, 0), (0, 0, -1))
        left = make_camera(center=(0, 0, 0))
        right = make_camera(center=(0, 0, 5), rotation=rot_flip)
        with self.assertRaises(sr.GeometryError):
            sr.triangulate(left, (320.0, 240.0), right, (320.0, 240.0), 1.0)

    def test_threshold_validation(self):
        with self.assertRaises(ValueError):
            sr.triangulate(self.left, (0, 0), self.right, (0, 0), 0.0)
        with self.assertRaises(ValueError):
            sr.triangulate(self.left, (0, 0), self.right, (0, 0), 90.0)


class TestPipeline(unittest.TestCase):
    def test_run_end_to_end(self):
        left = make_camera(center=(0, 0, 0), k1=0.1)
        right = make_camera(center=(1, 0, 0))
        point = (0.5, 0.1, 4.0)
        ul, vl, _ = left.project(point)
        ur, vr, _ = right.project(point)

        with tempfile.TemporaryDirectory() as tmp:
            cams = {
                "cameras": {
                    "left": {"fx": 800, "fy": 820, "cx": 320, "cy": 240,
                             "k1": 0.1, "k2": 0.0,
                             "center": [0, 0, 0],
                             "rotation": [[1, 0, 0], [0, 1, 0], [0, 0, 1]]},
                    "right": {"fx": 800, "fy": 820, "cx": 320, "cy": 240,
                              "k1": 0.0, "k2": 0.0,
                              "center": [1, 0, 0],
                              "rotation": [[1, 0, 0], [0, 1, 0], [0, 0, 1]]},
                }
            }
            cj = os.path.join(tmp, "cams.json")
            with open(cj, "w", encoding="utf-8") as fh:
                json.dump(cams, fh)
            csv_path = os.path.join(tmp, "pts.csv")
            with open(csv_path, "w", encoding="utf-8", newline="") as fh:
                fh.write("point_id,left_u,left_v,right_u,right_v\n")
                fh.write(f"p1,{ul},{vl},{ur},{vr}\n")
                fh.write("bad,400,240,400,240\n")  # parallel rays -> failure
            out = os.path.join(tmp, "report.json")
            report = sr.run(cj, csv_path, out, 1.0)
            self.assertEqual(report["summary"]["ok"], 1)
            self.assertEqual(report["summary"]["failed"], 1)
            ok = [p for p in report["points"] if p["id"] == "p1"][0]
            self.assertEqual(ok["status"], "ok")
            for got, want in zip(ok["point"], point):
                self.assertAlmostEqual(got, want, places=9)
            bad = [p for p in report["points"] if p["id"] == "bad"][0]
            self.assertEqual(bad["status"], "failed")
            self.assertTrue(os.path.exists(out))

    def test_duplicate_ids_rejected(self):
        with tempfile.TemporaryDirectory() as tmp:
            csv_path = os.path.join(tmp, "pts.csv")
            with open(csv_path, "w", encoding="utf-8") as fh:
                fh.write("a,1,2,3,4\na,1,2,3,4\n")
            with self.assertRaises(ValueError):
                sr.read_matches(csv_path)


if __name__ == "__main__":
    unittest.main()
