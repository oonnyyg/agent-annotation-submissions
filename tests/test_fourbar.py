import math
import unittest

from fourbar.core import (
    AMBIGUOUS,
    OK,
    SINGULAR,
    UNREACHABLE,
    Linkage,
    analyze,
)


def cross(ax, ay, bx, by):
    return ax * by - ay * bx


class PositionTest(unittest.TestCase):
    def setUp(self):
        self.link = Linkage(ground=4.0, crank=1.0, coupler=3.0, rocker=2.5)

    def test_two_branches_mirror_and_sign(self):
        theta = 0.7
        plus = analyze(self.link, theta, 1.0, +1)
        minus = analyze(self.link, theta, 1.0, -1)
        self.assertEqual(plus.status, OK)
        self.assertEqual(minus.status, OK)
        ax, ay = plus.a
        for branch, cfg in ((+1, plus), (-1, minus)):
            bx, by = cfg.b
            s = cross(self.link.ground - ax, -ay, bx - ax, by - ay)
            self.assertEqual(math.copysign(1.0, s), float(branch))
        # two branches share the A->O2 axis foot: mirrored across line A-O2
        self.assertNotAlmostEqual(plus.b[1], minus.b[1])
        self.assertAlmostEqual(plus.a[0], minus.a[0])

    def test_branch_consistent_across_sweep(self):
        for i in range(50):
            theta = 2.0 * math.pi * i / 50.0
            cfg = analyze(self.link, theta, 1.0, +1)
            if cfg.status != OK:
                continue
            ax, ay = cfg.a
            bx, by = cfg.b
            s = cross(self.link.ground - ax, -ay, bx - ax, by - ay)
            self.assertGreater(s, 0.0, msg=f"branch flip at theta={theta}")

    def test_tangent_is_singular_unique(self):
        # crank=1, theta=0 -> d=3 = coupler+rocker (2+1): externally tangent
        link = Linkage(ground=4.0, crank=1.0, coupler=2.0, rocker=1.0)
        cfg = analyze(link, 0.0, 1.0, +1)
        self.assertEqual(cfg.status, SINGULAR)
        self.assertIsNotNone(cfg.b)
        self.assertIsNone(cfg.omega_coupler)
        self.assertIsNone(cfg.omega_rocker)
        self.assertIsNone(cfg.b_vel)
        # B lies on segment A->O2 at distance coupler from A
        self.assertAlmostEqual(cfg.b[0], 1.0 + 2.0)
        self.assertAlmostEqual(cfg.b[1], 0.0)

    def test_unreachable_separate(self):
        link = Linkage(ground=10.0, crank=1.0, coupler=2.0, rocker=1.0)
        cfg = analyze(link, 0.0, 1.0, +1)
        self.assertEqual(cfg.status, UNREACHABLE)
        self.assertIsNone(cfg.b)

    def test_unreachable_nested(self):
        link = Linkage(ground=0.5, crank=0.2, coupler=0.3, rocker=5.0)
        cfg = analyze(link, 0.0, 1.0, +1)
        self.assertEqual(cfg.status, UNREACHABLE)

    def test_coincident_ambiguous(self):
        link = Linkage(ground=2.0, crank=2.0, coupler=1.5, rocker=1.5)
        cfg = analyze(link, 0.0, 1.0, +1)
        self.assertEqual(cfg.status, AMBIGUOUS)
        self.assertIsNone(cfg.b)


class VelocityTest(unittest.TestCase):
    def setUp(self):
        self.link = Linkage(ground=4.0, crank=1.0, coupler=3.0, rocker=2.5)

    def test_residuals_near_zero(self):
        for i in range(1, 12):
            theta = 0.3 * i
            cfg = analyze(self.link, theta, 2.0, +1)
            if cfg.status != OK:
                continue
            for r in cfg.length_residuals:
                self.assertAlmostEqual(r, 0.0, places=9)
            for r in cfg.velocity_residuals:
                self.assertAlmostEqual(r, 0.0, places=7)

    def test_velocity_matches_finite_difference(self):
        theta, omega = 1.1, 1.5
        dt = 1e-6
        c0 = analyze(self.link, theta, omega, +1)
        c1 = analyze(self.link, theta + dt, omega, +1)
        self.assertEqual(c0.status, OK)
        self.assertEqual(c1.status, OK)
        num_vx = omega * (c1.b[0] - c0.b[0]) / dt
        num_vy = omega * (c1.b[1] - c0.b[1]) / dt
        self.assertAlmostEqual(c0.b_vel[0], num_vx, delta=1e-4)
        self.assertAlmostEqual(c0.b_vel[1], num_vy, delta=1e-4)

    def test_singular_has_no_velocity(self):
        link = Linkage(ground=4.0, crank=1.0, coupler=2.0, rocker=1.0)
        cfg = analyze(link, 0.0, 3.0, -1)
        self.assertEqual(cfg.status, SINGULAR)
        self.assertIsNone(cfg.b_vel)


class ProbeTest(unittest.TestCase):
    def test_probe_at_coupler_end_equals_b(self):
        link = Linkage(ground=4.0, crank=1.0, coupler=3.0, rocker=2.5)
        cfg = analyze(link, 0.9, 2.0, -1, probe=(3.0, 0.0))
        self.assertEqual(cfg.status, OK)
        self.assertAlmostEqual(cfg.probe_pos[0], cfg.b[0], places=9)
        self.assertAlmostEqual(cfg.probe_pos[1], cfg.b[1], places=9)
        self.assertAlmostEqual(cfg.probe_vel[0], cfg.b_vel[0], places=9)
        self.assertAlmostEqual(cfg.probe_vel[1], cfg.b_vel[1], places=9)

    def test_probe_keeps_local_coords(self):
        link = Linkage(ground=4.0, crank=1.0, coupler=3.0, rocker=2.5)
        u, v = 1.2, 0.4
        cfg = analyze(link, 2.0, 1.0, +1, probe=(u, v))
        self.assertEqual(cfg.status, OK)
        ax, ay = cfg.a
        bx, by = cfg.b
        L = math.hypot(bx - ax, by - ay)
        e1 = ((bx - ax) / L, (by - ay) / L)
        e2 = (-e1[1], e1[0])
        px, py = cfg.probe_pos
        rx, ry = px - ax, py - ay
        self.assertAlmostEqual(rx * e1[0] + ry * e1[1], u, places=9)
        self.assertAlmostEqual(rx * e2[0] + ry * e2[1], v, places=9)


if __name__ == "__main__":
    unittest.main()
