"""Tests for the compound gear kinematics checker.

Covers planetary rows with different locked members, conflicting drivers,
underdetermined cases, ordering independence and power balance.
"""

import math
import unittest

from gear_kinematics.__main__ import run
from gear_kinematics.model import ModelError, load_train


def planetary_train(cases, connections=None):
    conns = connections if connections is not None else [
        {"type": "planetary", "label": "pg",
         "sun": {"shaft": "sun", "teeth": 30},
         "ring": {"shaft": "ring", "teeth": 90},
         "carrier": "carrier"},
    ]
    return {"shafts": ["sun", "ring", "carrier"],
            "connections": conns, "cases": cases}


def case_diag(diag, name):
    return next(c for c in diag["cases"] if c["name"] == name)


def speeds_of(csv_rows, case_name):
    return {r[1]: float(r[2]) for r in csv_rows[1:]
            if r[0] == case_name and r[2] != ""}


class PlanetaryTests(unittest.TestCase):
    def test_ring_fixed(self):
        # Willis: z_s*w_s + z_r*w_r = (z_s+z_r)*w_c; w_r=0, w_s=120 => w_c=30
        data = planetary_train([{"name": "c1", "speeds": {"sun": 120},
                                 "fixed": ["ring"]}])
        csv_rows, diag = run(data)
        self.assertEqual(case_diag(diag, "c1")["status"], "unique")
        s = speeds_of(csv_rows, "c1")
        self.assertAlmostEqual(s["carrier"], 30.0)
        self.assertAlmostEqual(s["ring"], 0.0)

    def test_carrier_fixed(self):
        # w_s*z_s + w_r*z_r = 0 => w_r = -120*30/90 = -40
        data = planetary_train([{"name": "c2", "speeds": {"sun": 120},
                                 "fixed": ["carrier"]}])
        csv_rows, diag = run(data)
        self.assertEqual(case_diag(diag, "c2")["status"], "unique")
        s = speeds_of(csv_rows, "c2")
        self.assertAlmostEqual(s["ring"], -40.0)

    def test_sun_fixed(self):
        # w_r*z_r = (z_s+z_r)*w_c => w_c = 90*60/120 = 45
        data = planetary_train([{"name": "c3", "speeds": {"ring": 60},
                                 "fixed": ["sun"]}])
        csv_rows, diag = run(data)
        s = speeds_of(csv_rows, "c3")
        self.assertAlmostEqual(s["carrier"], 45.0)

    def test_residuals_zero(self):
        data = planetary_train([{"name": "c", "speeds": {"sun": 100},
                                 "fixed": ["ring"]}])
        _, diag = run(data)
        for r in case_diag(diag, "c")["residuals"]:
            self.assertAlmostEqual(r["residual"], 0.0, places=7)

    def test_bad_geometry_rejected(self):
        conns = [{"type": "planetary",
                  "sun": {"shaft": "sun", "teeth": 30},
                  "ring": {"shaft": "ring", "teeth": 91},
                  "carrier": "carrier"}]
        with self.assertRaises(ModelError):
            load_train(planetary_train([], connections=conns))

    def test_bad_teeth_rejected(self):
        conns = [{"type": "external_mesh",
                  "gear1": {"shaft": "sun", "teeth": 0},
                  "gear2": {"shaft": "ring", "teeth": 20}}]
        with self.assertRaises(ModelError):
            load_train(planetary_train([], connections=conns))


class CompoundTrainTests(unittest.TestCase):
    def train(self, order=(0, 1, 2)):
        conns = [
            {"type": "external_mesh", "label": "m1",
             "gear1": {"shaft": "in", "teeth": 20},
             "gear2": {"shaft": "mid", "teeth": 40}},
            {"type": "rigid", "label": "r1", "shafts": ["mid", "mid2"]},
            {"type": "internal_mesh", "label": "m2",
             "gear1": {"shaft": "mid2", "teeth": 15},
             "gear2": {"shaft": "out", "teeth": 60}},
        ]
        return {"shafts": ["in", "mid", "mid2", "out"],
                "connections": [conns[i] for i in order],
                "cases": [{"name": "run", "speeds": {"in": 200},
                           "torques": {"in": 10.0, "out": -400.0}}]}

    def test_compound_solution(self):
        # mid = -200*20/40 = -100; out = -100*15/60 = -25 (internal: same sign)
        csv_rows, diag = run(self.train())
        self.assertEqual(case_diag(diag, "run")["status"], "unique")
        s = speeds_of(csv_rows, "run")
        self.assertAlmostEqual(s["mid"], -100.0)
        self.assertAlmostEqual(s["mid2"], -100.0)
        self.assertAlmostEqual(s["out"], -25.0)

    def test_order_independent(self):
        _, d1 = run(self.train((0, 1, 2)))
        _, d2 = run(self.train((2, 0, 1)))
        p1 = case_diag(d1, "run")["power"]["per_shaft"]
        p2 = case_diag(d2, "run")["power"]["per_shaft"]
        for shaft in p1:
            self.assertAlmostEqual(p1[shaft]["speed_rpm"], p2[shaft]["speed_rpm"])

    def test_power_balance(self):
        _, diag = run(self.train())
        power = case_diag(diag, "run")["power"]
        p_in = 10.0 * 200 * 2 * math.pi / 60
        p_out = -400.0 * (-25.0) * 2 * math.pi / 60
        self.assertAlmostEqual(power["per_shaft"]["in"]["power_W"], p_in)
        self.assertAlmostEqual(power["per_shaft"]["out"]["power_W"], p_out)
        self.assertAlmostEqual(power["total_power_W"], p_in + p_out)
        self.assertEqual(power["per_shaft"]["mid"]["power_W"], 0.0)


class ConstraintStatusTests(unittest.TestCase):
    def test_conflict_reports_sources(self):
        data = {"shafts": ["a", "b"],
                "connections": [{"type": "rigid", "label": "rigid-ab",
                                 "shafts": ["a", "b"]}],
                "cases": [{"name": "bad",
                           "speeds": {"a": 100, "b": 50},
                           "torques": {"a": 5.0}}]}
        csv_rows, diag = run(data)
        entry = case_diag(diag, "bad")
        self.assertEqual(entry["status"], "conflicting")
        self.assertIn("rigid-ab", entry["conflict_sources"])
        self.assertNotIn("power", entry)
        self.assertEqual(speeds_of(csv_rows, "bad"), {})

    def test_consistent_duplicate_allowed(self):
        data = {"shafts": ["a", "b"],
                "connections": [{"type": "rigid", "label": "r1",
                                 "shafts": ["a", "b"]}],
                "cases": [{"name": "ok", "speeds": {"a": 100, "b": 100}}]}
        _, diag = run(data)
        self.assertEqual(case_diag(diag, "ok")["status"], "unique")

    def test_underdetermined_reports_freedom(self):
        data = {"shafts": ["a", "b", "c"],
                "connections": [{"type": "external_mesh", "label": "m",
                                 "gear1": {"shaft": "a", "teeth": 20},
                                 "gear2": {"shaft": "b", "teeth": 20}}],
                "cases": [{"name": "loose", "speeds": {"a": 60},
                           "torques": {"c": 3.0}}]}
        csv_rows, diag = run(data)
        entry = case_diag(diag, "loose")
        self.assertEqual(entry["status"], "underdetermined")
        self.assertEqual(entry["degrees_of_freedom"], 1)
        self.assertEqual(entry["free_shafts"], ["c"])
        self.assertNotIn("power", entry)
        self.assertEqual(speeds_of(csv_rows, "loose"), {})

    def test_cases_independent(self):
        data = planetary_train([
            {"name": "good", "speeds": {"sun": 120}, "fixed": ["ring"]},
            {"name": "bad", "speeds": {"sun": 10, "ring": 5, "carrier": 1}},
        ])
        _, diag = run(data)
        self.assertEqual(case_diag(diag, "good")["status"], "unique")
        self.assertEqual(case_diag(diag, "bad")["status"], "conflicting")


if __name__ == "__main__":
    unittest.main()
