import json
import os
import subprocess
import sys
import tempfile
import unittest

from hull_hydrostatics import solver


def rect(w=2.0, h=1.0):
    return [(-w / 2, 0.0), (w / 2, 0.0), (w / 2, h), (-w / 2, h)]


class SolverTests(unittest.TestCase):
    def test_rectangle_analytic_level(self):
        # Half-submerged 2x1 rectangle, rho=g=1, mass=1 (area target 1).
        res = solver.analyze_angle(rect(), (0.0, 0.5), 0.0, 1.0, 1.0, 1.0, 1e-9)
        self.assertEqual(res["status"], "ok")
        self.assertAlmostEqual(res["translation"], -0.5, places=9)
        self.assertAlmostEqual(res["buoyancy_center"][0], 0.0, places=9)
        self.assertAlmostEqual(res["buoyancy_center"][1], -0.25, places=9)
        self.assertAlmostEqual(res["gravity_center"][1], 0.0, places=9)
        self.assertAlmostEqual(res["displaced_mass"], 1.0, places=9)
        self.assertAlmostEqual(res["equilibrium_residual"], 0.0, places=6)
        self.assertEqual(res["behaviour"], "neutral")

    def test_rectangle_heeled_righting(self):
        # CG on the centreline: a heeled symmetric rectangle must
        # produce a moment opposing the heel (righting).
        res = solver.analyze_angle(rect(), (0.0, 0.5), 15.0, 1.0, 1.0, 9.81, 1e-9)
        self.assertEqual(res["status"], "ok")
        self.assertLess(res["moment_about_cg"], 0.0)
        self.assertEqual(res["behaviour"], "righting")
        neg = solver.analyze_angle(rect(), (0.0, 0.5), -15.0, 1.0, 1.0, 9.81, 1e-9)
        self.assertGreater(neg["moment_about_cg"], 0.0)
        self.assertEqual(neg["behaviour"], "righting")

    def test_aggravating_when_cg_offset(self):
        # Positive (CCW) heel immerses the port side; a CG offset to
        # port then adds to the inclining moment -> aggravating.
        res = solver.analyze_angle(rect(), (-0.95, 0.5), 10.0, 1.0, 1.0, 9.81, 1e-9)
        self.assertGreater(res["moment_about_cg"], 0.0)
        self.assertEqual(res["behaviour"], "aggravating")

    def test_order_independence(self):
        a = solver.analyze_angle(rect(), (0.0, 0.5), 20.0, 1.0, 1.0, 9.81, 1e-9)
        b = solver.analyze_angle(rect(), (0.0, 0.5), 20.0, 1.0, 1.0, 9.81, 1e-9)
        self.assertEqual(a, b)


class CliTests(unittest.TestCase):
    def _run(self, cfg):
        tmp = tempfile.mkdtemp()
        cfg_path = os.path.join(tmp, "cfg.json")
        csv_path = os.path.join(tmp, "out.csv")
        diag_path = os.path.join(tmp, "diag.json")
        with open(cfg_path, "w", encoding="utf-8") as fh:
            json.dump(cfg, fh)
        proc = subprocess.run(
            [sys.executable, "-m", "hull_hydrostatics", cfg_path,
             "--csv", csv_path, "--diag", diag_path],
            capture_output=True, text=True)
        return proc, csv_path, diag_path

    def test_cli_rectangle(self):
        cfg = {
            "polygon": rect(),
            "center_of_gravity": [0.0, 0.5],
            "unit_length_mass": 1.0,
            "fluid_density": 1.0,
            "gravity": 9.81,
            "angles_deg": [0.0, 10.0, -10.0],
        }
        proc, csv_path, diag_path = self._run(cfg)
        self.assertEqual(proc.returncode, 0, proc.stderr)
        with open(csv_path, encoding="utf-8") as fh:
            lines = fh.read().strip().splitlines()
        self.assertEqual(len(lines), 4)
        self.assertIn("righting", lines[2])
        with open(diag_path, encoding="utf-8") as fh:
            diag = json.load(fh)
        self.assertAlmostEqual(diag["section_area"], 2.0)
        self.assertEqual(len(diag["angles"]), 3)

    def test_cli_rejects_bad_mass(self):
        cfg = {
            "polygon": rect(),
            "center_of_gravity": [0.0, 0.5],
            "unit_length_mass": 5.0,
            "fluid_density": 1.0,
            "gravity": 9.81,
            "angles_deg": [0.0],
        }
        proc, _, _ = self._run(cfg)
        self.assertEqual(proc.returncode, 2)
        self.assertIn("unit_length_mass", proc.stderr)


if __name__ == "__main__":
    unittest.main()
