import json
import math
import os
import sys
import tempfile
import unittest
sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
from gyro_replay import GyroReplay, Quaternion
from gyro_replay.cli import main as cli_main


class QuaternionTests(unittest.TestCase):
    def test_zero_quaternion_rejected(self):
        with self.assertRaises(ValueError):
            Quaternion(0, 0, 0, 0).normalized()
        with self.assertRaises(ValueError):
            GyroReplay([0, 1], [(0, 0, 0)], q0=Quaternion(0, 0, 0, 0))

    def test_nonunit_input_normalized(self):
        q = Quaternion(2, 0, 0, 0).normalized()
        self.assertAlmostEqual(q.norm(), 1.0)

    def test_axis_order_not_commutative(self):
        # 90 deg about x then y differs from y then x.
        qx = Quaternion.from_axis_angle((1, 0, 0), math.pi / 2)
        qy = Quaternion.from_axis_angle((0, 1, 0), math.pi / 2)
        err = (qy * qx).attitude_error(qx * qy)
        self.assertGreater(err, 1.0)

    def test_sign_ambiguity_zero_error(self):
        q = Quaternion.from_axis_angle((0, 0, 1), 1.0)
        self.assertAlmostEqual(q.attitude_error(Quaternion(-q.w, -q.x, -q.y, -q.z)), 0.0)

    def test_rotate_vector(self):
        q = Quaternion.from_axis_angle((0, 0, 1), math.pi / 2)
        v = q.rotate_vector((1, 0, 0))
        self.assertAlmostEqual(v[0], 0.0, places=12)
        self.assertAlmostEqual(v[1], 1.0, places=12)


class IntegratorTests(unittest.TestCase):
    def test_zero_rate_keeps_attitude(self):
        r = GyroReplay([0.0, 1.0, 2.0], [(0, 0, 0), (0, 0, 0)])
        self.assertEqual(r.attitude(1.5), Quaternion.identity())

    def test_exact_single_axis(self):
        r = GyroReplay([0.0, 1.0], [(0, 0, 1.0)])
        q = r.attitude(1.0)
        self.assertAlmostEqual(q.attitude_error(
            Quaternion.from_axis_angle((0, 0, 1), 1.0)), 0.0, places=12)

    def test_mid_segment_query(self):
        r = GyroReplay([0.0, 2.0], [(0, 0, 1.0)])
        q = r.attitude(0.5)
        self.assertAlmostEqual(q.attitude_error(
            Quaternion.from_axis_angle((0, 0, 1), 0.5)), 0.0, places=12)

    def test_long_rotation_not_shortest_arc(self):
        # One segment rotates 3*pi/2 about z; midpoint must be at 3*pi/4,
        # not a shortest-arc interpolation between endpoints.
        r = GyroReplay([0.0, 1.0], [(0, 0, 1.5 * math.pi)])
        q_mid = r.attitude(0.5)
        expect = Quaternion.from_axis_angle((0, 0, 1), 0.75 * math.pi)
        self.assertAlmostEqual(q_mid.attitude_error(expect), 0.0, places=12)
        q_end = r.attitude(1.0)
        expect_end = Quaternion.from_axis_angle((0, 0, 1), 1.5 * math.pi)
        self.assertAlmostEqual(q_end.attitude_error(expect_end), 0.0, places=12)

    def test_bias_subtracted(self):
        # Bias cancels the measured rate -> no rotation.
        r = GyroReplay([0.0, 1.0], [(0.1, -0.2, 0.3)], bias=(0.1, -0.2, 0.3))
        self.assertAlmostEqual(
            r.attitude(1.0).attitude_error(Quaternion.identity()), 0.0)

    def test_rotation_order_preserved(self):
        # Segment 1: 90 deg about x; segment 2: 90 deg about (new) body y.
        # Right-multiplied body increments compose as qx * qy.
        r = GyroReplay([0.0, 1.0, 2.0],
                       [(math.pi / 2, 0, 0), (0, math.pi / 2, 0)])
        qx = Quaternion.from_axis_angle((1, 0, 0), math.pi / 2)
        qy = Quaternion.from_axis_angle((0, 1, 0), math.pi / 2)
        self.assertAlmostEqual(
            r.attitude(2.0).attitude_error(qx * qy), 0.0, places=12)
        self.assertGreater(
            r.attitude(2.0).attitude_error(qy * qx), 1.0)

    def test_no_extrapolation(self):
        r = GyroReplay([0.0, 1.0], [(0, 0, 1.0)])
        with self.assertRaises(ValueError):
            r.attitude(1.5)
        with self.assertRaises(ValueError):
            r.attitude(-0.1)

    def test_unit_norm_maintained(self):
        r = GyroReplay([0.0, 0.3, 0.7, 1.0],
                       [(1, 2, 3), (-2, 1, 0.5), (0.1, -0.1, 2)])
        for t in (0.1, 0.3, 0.55, 0.7, 0.99, 1.0):
            self.assertAlmostEqual(r.attitude(t).norm(), 1.0, places=12)


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        with tempfile.TemporaryDirectory() as d:
            gyro = os.path.join(d, "gyro.csv")
            bias = os.path.join(d, "bias.json")
            ref = os.path.join(d, "ref.csv")
            out = os.path.join(d, "att.csv")
            errj = os.path.join(d, "err.json")
            with open(gyro, "w") as fh:
                fh.write("t,wx,wy,wz\n0,0,0,1.1\n1,0,0,1.1\n2,0,0,1.1\n")
            with open(bias, "w") as fh:
                json.dump({"wx": 0.0, "wy": 0.0, "wz": 0.1}, fh)
            q1 = Quaternion.from_axis_angle((0, 0, 1), 1.0)
            q2 = Quaternion.from_axis_angle((0, 0, 1), 2.0)
            with open(ref, "w") as fh:
                fh.write(f"1,{q1.w},{q1.x},{q1.y},{q1.z}\n")
                fh.write(f"2,{-q2.w},{-q2.x},{-q2.y},{-q2.z}\n")
            rc = cli_main([gyro, "--bias-json", bias, "--reference-csv", ref,
                           "--out", out, "--error-json", errj,
                           "--vector", "1,0,0"])
            self.assertEqual(rc, 0)
            with open(errj) as fh:
                summary = json.load(fh)
            self.assertEqual(summary["count"], 2)
            self.assertLess(summary["max_rad"], 1e-9)
            with open(out) as fh:
                lines = fh.read().strip().splitlines()
            self.assertEqual(len(lines), 4)
            self.assertIn("vx_world", lines[0])


if __name__ == "__main__":
    unittest.main()
