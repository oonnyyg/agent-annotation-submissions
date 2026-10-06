import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from hull_hydrostatics.geometry import (
    GeometryError,
    area_centroid,
    clip_below_waterline,
    rotate_points,
    translate_points,
    validate_polygon,
)
from hull_hydrostatics.solver import analyze_angle
from hull_hydrostatics.__main__ import load_config, run


class TestGeometry(unittest.TestCase):
    def test_validate_accepts_both_windings(self):
        ccw = [(0, 0), (2, 0), (2, 1), (0, 1)]
        cw = list(reversed(ccw))
        self.assertEqual(validate_polygon(ccw), validate_polygon(cw))

    def test_validate_accepts_concave(self):
        poly = [(0, 0), (3, 0), (3, 3), (2, 3), (2, 1), (1, 1), (1, 3), (0, 3)]
        result = validate_polygon(poly)
        area, _ = area_centroid(result)
        self.assertAlmostEqual(area, 7.0)

    def test_validate_rejects_self_intersection(self):
        bowtie = [(0, 0), (2, 2), (2, 0), (0, 2)]
        with self.assertRaises(GeometryError):
            validate_polygon(bowtie)

    def test_validate_rejects_zero_area(self):
        with self.assertRaises(GeometryError):
            validate_polygon([(0, 0), (1, 0), (2, 0)])

    def test_clip_concave_waterline(self):
        # U-shape: two legs of width 1 and height 3, gap of width 1.
        poly = validate_polygon(
            [(0, 0), (3, 0), (3, 3), (2, 3), (2, 1), (1, 1), (1, 3), (0, 3)]
        )
        # Shift down by 1.5: submerges the full bottom bar (3x1) plus
        # 0.5 of each leg -> 3 + 2*0.5 = 4.0.
        shifted = translate_points(poly, 0.0, -1.5)
        submerged = clip_below_waterline(shifted)
        area, (cx, cy) = area_centroid(submerged)
        self.assertAlmostEqual(area, 4.0)
        self.assertAlmostEqual(cx, 1.5)
        # Bottom bar centroid y=-1.0 (area 3), legs centroid y=-0.25 (area 1).
        self.assertAlmostEqual(cy, (3.0 * -1.0 + 1.0 * -0.25) / 4.0)

    def test_clip_empty_when_above_water(self):
        poly = [(0, 1), (1, 1), (1, 2), (0, 2)]
        self.assertEqual(clip_below_waterline(poly), [])


class TestRectangleAnalytic(unittest.TestCase):
    # Rectangle 2 wide x 1 tall, CG at origin (center), half-loaded.
    def setUp(self):
        self.section = validate_polygon([(-1, -0.5), (1, -0.5), (1, 0.5), (-1, 0.5)])
        self.rho = 1000.0
        self.g = 9.81
        self.mass = 1000.0 * 1.0  # displaced area 1.0 -> draft 0.5
        self.target_area = self.mass / self.rho

    def test_zero_heel_translation_and_centers(self):
        r = analyze_angle(self.section, (0.0, 0.0), 0.0, self.target_area,
                          self.rho, self.g, 1e-9)
        self.assertEqual(r["status"], "ok")
        # Draft 0.5 over width 2 -> area 1.0; bottom at y=-0.5 must sit at
        # y=-0.5, so translation is 0.
        self.assertAlmostEqual(r["translation"], 0.0, places=8)
        self.assertAlmostEqual(r["buoyancy_center"][0], 0.0, places=8)
        self.assertAlmostEqual(r["buoyancy_center"][1], -0.25, places=8)
        self.assertAlmostEqual(r["gravity_center"][0], 0.0, places=8)
        self.assertAlmostEqual(r["gravity_center"][1], 0.0, places=8)
        self.assertAlmostEqual(r["displaced_mass"], self.mass, places=6)
        self.assertAlmostEqual(r["residual"], 0.0, places=6)
        self.assertEqual(r["classification"], "neutral")

    def test_heeled_symmetric_rectangle_righting(self):
        # CG below the section center -> righting moment when heeled.
        r = analyze_angle(self.section, (0.0, -0.2), 15.0, self.target_area,
                          self.rho, self.g, 1e-9)
        self.assertEqual(r["status"], "ok")
        self.assertAlmostEqual(r["residual"], 0.0, places=6)
        self.assertLess(r["moment_about_cg"], 0.0)  # opposite sign to +15 deg
        self.assertEqual(r["classification"], "righting")

    def test_heeled_high_cg_upsetting(self):
        r = analyze_angle(self.section, (0.0, 0.45), 15.0, self.target_area,
                          self.rho, self.g, 1e-9)
        self.assertEqual(r["status"], "ok")
        self.assertGreater(r["moment_about_cg"], 0.0)
        self.assertEqual(r["classification"], "upsetting")

    def test_moment_matches_horizontal_offset(self):
        cg = (0.0, -0.2)
        r = analyze_angle(self.section, cg, 20.0, self.target_area,
                          self.rho, self.g, 1e-9)
        expected = r["buoyancy"] * (r["buoyancy_center"][0] - r["gravity_center"][0])
        self.assertAlmostEqual(r["moment_about_cg"], expected)


class TestAngleIndependence(unittest.TestCase):
    def test_angle_order_does_not_matter(self):
        section = validate_polygon([(-1, -0.5), (1, -0.5), (1, 0.5), (-1, 0.5)])
        cfg = {
            "section": section,
            "mass_per_length": 900.0,
            "fluid_density": 1000.0,
            "gravity": 9.81,
            "center_of_gravity": (0.0, -0.1),
            "angles_deg": [-30.0, 0.0, 10.0, 45.0],
            "moment_tolerance": 1e-9,
            "full_area": 2.0,
            "section_centroid": (0.0, 0.0),
        }
        results_fwd, _ = run(cfg)
        cfg_rev = dict(cfg, angles_deg=list(reversed(cfg["angles_deg"])))
        results_rev, _ = run(cfg_rev)
        by_angle = {r["angle_deg"]: r for r in results_rev}
        for r in results_fwd:
            other = by_angle[r["angle_deg"]]
            self.assertEqual(r["status"], other["status"])
            if r["status"] == "ok":
                self.assertAlmostEqual(r["translation"], other["translation"])
                self.assertAlmostEqual(r["moment_about_cg"], other["moment_about_cg"])


class TestConfigValidation(unittest.TestCase):
    def _write(self, tmp, cfg):
        path = os.path.join(tmp, "cfg.json")
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(cfg, fh)
        return path

    def _base(self):
        return {
            "section": [[-1, -0.5], [1, -0.5], [1, 0.5], [-1, 0.5]],
            "mass_per_length": 1000.0,
            "fluid_density": 1000.0,
            "gravity": 9.81,
            "center_of_gravity": [0, 0],
            "angles_deg": [0, 10],
        }

    def test_mass_must_be_below_full_displacement(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = self._base()
            cfg["mass_per_length"] = 2000.0  # full displacement is 2000
            with self.assertRaises(ValueError):
                load_config(self._write(tmp, cfg))
            cfg["mass_per_length"] = 0.0
            with self.assertRaises(ValueError):
                load_config(self._write(tmp, cfg))

    def test_density_and_gravity_positive(self):
        with tempfile.TemporaryDirectory() as tmp:
            for key in ("fluid_density", "gravity"):
                cfg = self._base()
                cfg[key] = -1.0
                with self.assertRaises(ValueError):
                    load_config(self._write(tmp, cfg))


class TestCli(unittest.TestCase):
    def test_cli_end_to_end(self):
        with tempfile.TemporaryDirectory() as tmp:
            cfg = {
                "section": [[-1, -0.5], [1, -0.5], [1, 0.5], [-1, 0.5]],
                "mass_per_length": 1000.0,
                "fluid_density": 1000.0,
                "gravity": 9.81,
                "center_of_gravity": [0, -0.2],
                "angles_deg": [0, 10, -20],
            }
            cfg_path = os.path.join(tmp, "cfg.json")
            csv_path = os.path.join(tmp, "out.csv")
            diag_path = os.path.join(tmp, "diag.json")
            with open(cfg_path, "w", encoding="utf-8") as fh:
                json.dump(cfg, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "hull_hydrostatics", cfg_path,
                 "--csv", csv_path, "--diagnostics", diag_path],
                capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(csv_path, encoding="utf-8") as fh:
                lines = fh.read().strip().splitlines()
            self.assertEqual(len(lines), 4)  # header + 3 angles
            with open(diag_path, encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertAlmostEqual(diag["section_area"], 2.0)
            self.assertEqual(len(diag["angles"]), 3)


if __name__ == "__main__":
    unittest.main()
