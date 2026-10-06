import math
import unittest

from wirefield.coil import Coil, CoilDataError
from wirefield.probe import Probe, parse_probe_row, validate_basis
from wirefield.report import build_report
from wirefield.vector import norm


def rotated_probe(point, angle):
    c, s = math.cos(angle), math.sin(angle)
    return Probe(
        point=point,
        axis_u=(c, s, 0.0),
        axis_v=(-s, c, 0.0),
        axis_w=(0.0, 0.0, 1.0),
    )


COILS = [
    Coil("loop_a", ((-1, -1, 0), (1, -1, 0), (1, 1, 0), (-1, 1, 0)), 2.0),
    Coil("loop_b", ((-1, -1, 0), (1, -1, 0), (1, 1, 0), (-1, 1, 0)), 2.0),
]


class ProbeBasisTest(unittest.TestCase):
    def test_identity_basis(self):
        validate_basis((1, 0, 0), (0, 1, 0), (0, 0, 1), "t")

    def test_rejects_non_unit_axis(self):
        with self.assertRaises(CoilDataError):
            validate_basis((2, 0, 0), (0, 1, 0), (0, 0, 1), "t")

    def test_rejects_non_orthogonal(self):
        with self.assertRaises(CoilDataError):
            validate_basis((1, 0, 0), (1, 1, 0), (0, 0, 1), "t")

    def test_rejects_left_handed(self):
        with self.assertRaises(CoilDataError):
            validate_basis((1, 0, 0), (0, 1, 0), (0, 0, -1), "t")

    def test_row_parse(self):
        row = {
            "x": "0", "y": "0", "z": "0.5",
            "u1": "1", "u2": "0", "u3": "0",
            "v1": "0", "v2": "1", "v3": "0",
            "w1": "0", "w2": "0", "w3": "1",
        }
        probe = parse_probe_row(row, 2)
        self.assertEqual(probe.point, (0.0, 0.0, 0.5))


class RotationInvarianceTest(unittest.TestCase):
    def test_local_magnitude_matches_world_magnitude(self):
        world = (0.3, -0.4, 0.5)
        for angle in (0.0, 0.3, 1.1, 2.9):
            probe = rotated_probe((0, 0, 0), angle)
            local = probe.local_components(world)
            self.assertAlmostEqual(norm(local), norm(world), places=12)


class ReportTest(unittest.TestCase):
    def test_reversal_scenario_cancels_total(self):
        probe = Probe((0.3, -0.2, 0.7), (1, 0, 0), (0, 1, 0), (0, 0, 1))
        scenarios = [
            {"name": "reverse_b", "currents": {"loop_a": 2.0, "loop_b": -2.0}}
        ]
        report = build_report(COILS, [probe], scenarios, safe_distance=1e-3)
        point = report["points"][0]
        self.assertTrue(point["computable"])
        sc = point["scenarios"][0]
        self.assertAlmostEqual(sc["total_world"]["magnitude"], 0.0, places=15)
        base = point["baseline"]["total_world"]
        diff = sc["diff_world_vs_baseline"]
        self.assertAlmostEqual(diff["magnitude"], base["magnitude"], places=15)
        self.assertAlmostEqual(diff["z"], -base["z"], places=15)

    def test_safe_distance_blocks_point_and_names_coil(self):
        near = Probe((0.0, -1.0, 1e-4), (1, 0, 0), (0, 1, 0), (0, 0, 1))
        far = Probe((0.0, 0.0, 5.0), (1, 0, 0), (0, 1, 0), (0, 0, 1))
        report = build_report(COILS, [near, far], [], safe_distance=0.01)
        blocked, ok = report["points"]
        self.assertFalse(blocked["computable"])
        self.assertEqual(sorted(blocked["blocked_by"]), ["loop_a", "loop_b"])
        self.assertTrue(ok["computable"])
        self.assertLess(ok["baseline"]["total_world"]["magnitude"], 1e-6)

    def test_local_diff_reported_as_vector(self):
        probe = rotated_probe((0.0, 0.0, 0.5), 0.7)
        scenarios = [{"name": "half", "currents": {"loop_a": 1.0, "loop_b": 1.0}}]
        report = build_report(COILS, [probe], scenarios, safe_distance=1e-3)
        sc = report["points"][0]["scenarios"][0]
        dl = sc["diff_local_vs_baseline"]
        dw = sc["diff_world_vs_baseline"]
        dl_norm = math.sqrt(dl["x"] ** 2 + dl["y"] ** 2 + dl["z"] ** 2)
        self.assertAlmostEqual(dl_norm, dw["magnitude"], places=15)


if __name__ == "__main__":
    unittest.main()
