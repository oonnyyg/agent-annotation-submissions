import json
import math
import os
import tempfile
import unittest

from gyro_replay import (
    AttitudeReplay,
    Quaternion,
    load_bias_json,
    load_gyro_csv,
    load_reference_csv,
)

HALF_PI = math.pi / 2.0


def q_axis(axis, angle):
    return Quaternion.from_axis_angle(axis, angle)


class QuaternionTests(unittest.TestCase):
    def test_zero_quaternion_rejected(self):
        with self.assertRaises(ValueError):
            Quaternion.from_sequence([0.0, 0.0, 0.0, 0.0])

    def test_non_unit_input_normalized(self):
        q = Quaternion.from_sequence([2.0, 0.0, 0.0, 0.0])
        self.assertAlmostEqual(q.norm(), 1.0)
        self.assertAlmostEqual(q.w, 1.0)

    def test_sign_invariant_angle(self):
        q = q_axis((0, 0, 1), 0.7)
        neg = Quaternion(-q.w, -q.x, -q.y, -q.z)
        self.assertAlmostEqual(q.angle_to(neg), 0.0, places=12)

    def test_angle_range(self):
        a = q_axis((1, 0, 0), 0.3)
        b = q_axis((0, 1, 0), 2.9)
        err = a.angle_to(b)
        self.assertGreaterEqual(err, 0.0)
        self.assertLessEqual(err, math.pi)

    def test_rotate_vector(self):
        q = q_axis((0, 0, 1), HALF_PI)
        x, y, z = q.rotate((1.0, 0.0, 0.0))
        self.assertAlmostEqual(x, 0.0, places=12)
        self.assertAlmostEqual(y, 1.0, places=12)
        self.assertAlmostEqual(z, 0.0, places=12)

    def test_axis_order_not_swapped(self):
        # Right-multiplied body increments: q = qx * qy, not qy * qx.
        qx = q_axis((1, 0, 0), HALF_PI)
        qy = q_axis((0, 1, 0), HALF_PI)
        rows = [(0.0, (HALF_PI, 0.0, 0.0)), (1.0, (0.0, HALF_PI, 0.0)),
                (2.0, (0.0, 0.0, 0.0))]
        replay = AttitudeReplay(rows, Quaternion.identity())
        got = replay.attitude(2.0)
        self.assertAlmostEqual(got.angle_to(qx * qy), 0.0, places=12)
        self.assertGreater(got.angle_to(qy * qx), 0.1)

    def test_zero_rate_keeps_attitude(self):
        rows = [(0.0, (0.0, 0.0, 0.0)), (5.0, (0.0, 0.0, 0.0))]
        q0 = q_axis((1, 2, 3), 0.4)
        replay = AttitudeReplay(rows, q0)
        self.assertAlmostEqual(replay.attitude(2.5).angle_to(q0), 0.0, places=12)


class ReplayTests(unittest.TestCase):
    def test_bias_subtracted_before_integration(self):
        # True rate is zero; raw gyro reads the bias only.
        bias = (0.01, -0.02, 0.03)
        rows = [(0.0, bias), (10.0, bias)]
        replay = AttitudeReplay(rows, Quaternion.identity(), bias=bias)
        self.assertAlmostEqual(
            replay.attitude(10.0).angle_to(Quaternion.identity()), 0.0, places=12)

    def test_uncorrected_bias_drifts(self):
        bias = (0.01, 0.0, 0.0)
        rows = [(0.0, bias), (10.0, bias)]
        replay = AttitudeReplay(rows, Quaternion.identity())
        self.assertAlmostEqual(
            replay.attitude(10.0).angle_to(Quaternion.identity()), 0.1, places=9)

    def test_long_rotation_mid_segment(self):
        # One segment rotates 3*pi/2 about z in 1 s; the midpoint must be at
        # 3*pi/4 along the actual path, not on a shortest-arc interpolation.
        rate = 1.5 * math.pi
        rows = [(0.0, (0.0, 0.0, rate)), (1.0, (0.0, 0.0, 0.0))]
        replay = AttitudeReplay(rows, Quaternion.identity())
        mid = replay.attitude(0.5)
        expected = q_axis((0, 0, 1), 0.75 * math.pi)
        self.assertAlmostEqual(mid.angle_to(expected), 0.0, places=12)
        # Shortest-arc between endpoints would pass through identity-ish
        # attitudes; the real midpoint is 3*pi/4 away from identity.
        self.assertAlmostEqual(
            mid.angle_to(Quaternion.identity()), 0.75 * math.pi, places=12)
        end = replay.attitude(1.0)
        self.assertAlmostEqual(
            end.angle_to(q_axis((0, 0, 1), 1.5 * math.pi)), 0.0, places=12)

    def test_query_inside_segment(self):
        rows = [(0.0, (0.0, 0.0, 1.0)), (2.0, (0.0, 0.0, 0.0))]
        replay = AttitudeReplay(rows, Quaternion.identity())
        got = replay.attitude(0.25)
        self.assertAlmostEqual(
            got.angle_to(q_axis((0, 0, 1), 0.25)), 0.0, places=12)

    def test_query_outside_record_rejected(self):
        rows = [(0.0, (0.0, 0.0, 1.0)), (1.0, (0.0, 0.0, 0.0))]
        replay = AttitudeReplay(rows, Quaternion.identity())
        with self.assertRaises(ValueError):
            replay.attitude(-0.1)
        with self.assertRaises(ValueError):
            replay.attitude(1.1)

    def test_last_row_only_sets_end(self):
        rows = [(0.0, (0.0, 0.0, 1.0)), (1.0, (0.0, 0.0, 99.0))]
        replay = AttitudeReplay(rows, Quaternion.identity())
        self.assertAlmostEqual(
            replay.attitude(1.0).angle_to(q_axis((0, 0, 1), 1.0)), 0.0, places=12)

    def test_rotate_to_world(self):
        rows = [(0.0, (0.0, 0.0, HALF_PI)), (1.0, (0.0, 0.0, 0.0))]
        replay = AttitudeReplay(rows, Quaternion.identity())
        x, y, z = replay.rotate_to_world(1.0, (1.0, 0.0, 0.0))
        self.assertAlmostEqual(x, 0.0, places=12)
        self.assertAlmostEqual(y, 1.0, places=12)
        self.assertAlmostEqual(z, 0.0, places=12)


class LoaderTests(unittest.TestCase):
    def _write(self, text):
        fd, path = tempfile.mkstemp()
        with os.fdopen(fd, "w", encoding="utf-8") as fh:
            fh.write(text)
        self.addCleanup(os.unlink, path)
        return path

    def test_gyro_csv_requires_two_rows(self):
        path = self._write("0.0,0,0,0\n")
        with self.assertRaises(ValueError):
            load_gyro_csv(path)

    def test_gyro_csv_strictly_increasing(self):
        path = self._write("0.0,0,0,0\n0.0,0,0,0\n")
        with self.assertRaises(ValueError):
            load_gyro_csv(path)

    def test_gyro_csv_rejects_non_finite(self):
        path = self._write("0.0,0,0,0\n1.0,nan,0,0\n")
        with self.assertRaises(ValueError):
            load_gyro_csv(path)

    def test_gyro_csv_skips_comments_and_blanks(self):
        path = self._write("# header\n\n0.0,0,0,1\n1.0,0,0,0\n")
        rows = load_gyro_csv(path)
        self.assertEqual(len(rows), 2)
        self.assertEqual(rows[0][1], (0.0, 0.0, 1.0))

    def test_bias_json(self):
        path = self._write(json.dumps({"bias": [0.1, 0.2, 0.3]}))
        self.assertEqual(load_bias_json(path), (0.1, 0.2, 0.3))

    def test_bias_json_bad_shape(self):
        path = self._write(json.dumps({"bias": [0.1, 0.2]}))
        with self.assertRaises(ValueError):
            load_bias_json(path)

    def test_reference_csv_normalizes(self):
        path = self._write("0.5,2,0,0,0\n")
        ref = load_reference_csv(path)
        self.assertAlmostEqual(ref[0][1].norm(), 1.0)


class CliTests(unittest.TestCase):
    def test_cli_end_to_end(self):
        from gyro_replay.__main__ import main

        with tempfile.TemporaryDirectory() as tmp:
            gyro = os.path.join(tmp, "gyro.csv")
            with open(gyro, "w", encoding="utf-8") as fh:
                fh.write("0.0,0,0,1.0\n1.0,0,0,0\n")
            ref = os.path.join(tmp, "ref.csv")
            q = q_axis((0, 0, 1), 1.0)
            with open(ref, "w", encoding="utf-8") as fh:
                fh.write(f"1.0,{-q.w},{-q.x},{-q.y},{-q.z}\n")  # negated: same pose
            att = os.path.join(tmp, "att.csv")
            err = os.path.join(tmp, "err.json")
            rc = main([gyro, "--q0", "1,0,0,0", "--time", "0.5",
                       "--vector", "1,0,0", "--reference-csv", ref,
                       "--attitude-out", att, "--errors-out", err])
            self.assertEqual(rc, 0)
            with open(err, encoding="utf-8") as fh:
                summary = json.load(fh)
            self.assertEqual(summary["count"], 1)
            self.assertAlmostEqual(summary["max_rad"], 0.0, places=9)
            with open(att, encoding="utf-8") as fh:
                lines = fh.read().strip().splitlines()
            self.assertEqual(len(lines), 3)  # header + 2 gyro times

    def test_cli_rejects_bad_q0(self):
        from gyro_replay.__main__ import main

        with tempfile.TemporaryDirectory() as tmp:
            gyro = os.path.join(tmp, "gyro.csv")
            with open(gyro, "w", encoding="utf-8") as fh:
                fh.write("0.0,0,0,0\n1.0,0,0,0\n")
            self.assertEqual(main([gyro, "--q0", "0,0,0,0"]), 2)


if __name__ == "__main__":
    unittest.main()
