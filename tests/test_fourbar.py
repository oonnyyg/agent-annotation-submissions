import json
import math
import os
import subprocess
import sys
import tempfile
import unittest

from fourbar.core import (
    FourBar,
    STATUS_COINCIDENT,
    STATUS_OK,
    STATUS_SINGULAR,
    STATUS_UNREACHABLE,
    solve_configuration,
)


def cross(ax, ay, bx, by):
    return ax * by - ay * bx


class PositionTests(unittest.TestCase):
    def setUp(self):
        self.link = FourBar(r1=4.0, r2=1.0, r3=3.0, r4=2.5)

    def test_branch_sign_preserved(self):
        for branch in (1, -1):
            for k in range(8):
                theta = 0.4 * k
                res = solve_configuration(self.link, theta, 1.0, branch)
                if res.status != STATUS_OK:
                    continue
                bx, by = res.B
                cx, cy = res.C
                s = cross(4.0 - bx, 0.0 - by, cx - bx, cy - by)
                self.assertEqual(math.copysign(1.0, s), float(branch))

    def test_branches_are_mirror_images(self):
        # At theta = 0 the line B->D is the x-axis, so the two
        # intersections are mirror images about it.
        theta = 0.0
        plus = solve_configuration(self.link, theta, 1.0, +1)
        minus = solve_configuration(self.link, theta, 1.0, -1)
        self.assertEqual(plus.status, STATUS_OK)
        self.assertEqual(minus.status, STATUS_OK)
        self.assertAlmostEqual(plus.C[0], minus.C[0], places=12)
        self.assertAlmostEqual(plus.C[1], -minus.C[1], places=12)

    def test_unreachable_when_circles_separate(self):
        link = FourBar(r1=10.0, r2=1.0, r3=1.0, r4=1.0)
        res = solve_configuration(link, 0.0, 1.0, 1)
        self.assertEqual(res.status, STATUS_UNREACHABLE)
        self.assertIsNone(res.C)
        self.assertIsNone(res.omega3)

    def test_coincident_circles_reported(self):
        link = FourBar(r1=1.0, r2=1.0, r3=2.0, r4=2.0)
        res = solve_configuration(link, 0.0, 1.0, 1)  # B lands on D
        self.assertEqual(res.status, STATUS_COINCIDENT)

    def test_tangent_is_singular_with_position_only(self):
        # r3 + r4 == |B - D| exactly at theta = 0: external tangency.
        link = FourBar(r1=4.0, r2=1.0, r3=1.5, r4=1.5)
        res = solve_configuration(link, 0.0, 2.0, 1)
        self.assertEqual(res.status, STATUS_SINGULAR)
        self.assertIsNotNone(res.C)
        self.assertAlmostEqual(res.C[0], 2.5, places=12)
        self.assertAlmostEqual(res.C[1], 0.0, places=12)
        self.assertIsNone(res.vC)
        self.assertIsNone(res.omega3)
        self.assertIsNone(res.omega4)

    def test_invalid_lengths_rejected(self):
        with self.assertRaises(ValueError):
            FourBar(r1=4.0, r2=-1.0, r3=3.0, r4=2.5)


class VelocityTests(unittest.TestCase):
    def setUp(self):
        self.link = FourBar(r1=4.0, r2=1.0, r3=3.0, r4=2.5)

    def test_residuals_near_zero(self):
        for k in range(12):
            theta = 0.3 * k
            res = solve_configuration(self.link, theta, 1.7, 1)
            if res.status != STATUS_OK:
                continue
            for value in res.length_residuals + res.velocity_residuals:
                self.assertAlmostEqual(value, 0.0, places=9)

    def test_velocity_matches_finite_difference(self):
        theta, omega = 1.1, 2.0
        dt = 1e-6
        r0 = solve_configuration(self.link, theta, omega, 1)
        r1 = solve_configuration(self.link, theta + omega * dt, omega, 1)
        self.assertEqual(r0.status, STATUS_OK)
        self.assertEqual(r1.status, STATUS_OK)
        num_vcx = (r1.C[0] - r0.C[0]) / dt
        num_vcy = (r1.C[1] - r0.C[1]) / dt
        self.assertAlmostEqual(r0.vC[0], num_vcx, places=4)
        self.assertAlmostEqual(r0.vC[1], num_vcy, places=4)

    def test_probe_point_and_velocity(self):
        theta, omega = 0.9, 1.3
        probe = (1.5, 0.5)
        dt = 1e-6
        r0 = solve_configuration(self.link, theta, omega, 1, probe)
        r1 = solve_configuration(self.link, theta + omega * dt, omega, 1, probe)
        self.assertEqual(r0.status, STATUS_OK)
        # Probe is rigidly attached to the coupler: |P - B| constant.
        d0 = math.dist(r0.probe, r0.B)
        d1 = math.dist(r1.probe, r1.B)
        self.assertAlmostEqual(d0, d1, places=9)
        num_vx = (r1.probe[0] - r0.probe[0]) / dt
        num_vy = (r1.probe[1] - r0.probe[1]) / dt
        self.assertAlmostEqual(r0.probe_velocity[0], num_vx, places=4)
        self.assertAlmostEqual(r0.probe_velocity[1], num_vy, places=4)


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        payload = {
            "links": {"r1": 4.0, "r2": 1.0, "r3": 3.0, "r4": 2.5},
            "branch": -1,
            "angles": [0.0, 0.5, 1.0],
            "omegas": [1.0, 1.0, 1.0],
            "probe": {"u": 1.0, "v": 0.25},
        }
        with tempfile.TemporaryDirectory() as tmp:
            src = os.path.join(tmp, "in.json")
            with open(src, "w", encoding="utf-8") as fh:
                json.dump(payload, fh)
            prefix = os.path.join(tmp, "out")
            proc = subprocess.run(
                [sys.executable, "-m", "fourbar", src, "-o", prefix],
                capture_output=True, text=True,
                cwd=os.path.dirname(os.path.dirname(os.path.abspath(__file__))),
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(prefix + ".csv", encoding="utf-8") as fh:
                lines = fh.read().strip().splitlines()
            self.assertEqual(len(lines), 4)  # header + 3 rows
            with open(prefix + ".diagnostics.json", encoding="utf-8") as fh:
                diag = json.load(fh)
            self.assertTrue(diag["all_ok"])
            self.assertEqual(diag["branch"], -1)
            self.assertLess(diag["max_abs_velocity_residual"], 1e-9)


if __name__ == "__main__":
    unittest.main()
