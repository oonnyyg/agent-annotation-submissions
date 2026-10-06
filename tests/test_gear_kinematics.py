import json
import os
import subprocess
import sys
import tempfile
import unittest

from gear_kinematics.model import load_document
from gear_kinematics.errors import InputError
from gear_kinematics.solver import solve
from gear_kinematics.__main__ import run_case, RPM_TO_RAD_S

# Simple planetary set: sun 24, planet 24, ring 72 (24 + 2*24 == 72).
PLANETARY = {
    "axes": ["sun", "ring", "carrier"],
    "connections": [
        {"id": "p1", "type": "planetary",
         "sun": {"axis": "sun", "teeth": 24},
         "ring": {"axis": "ring", "teeth": 72},
         "carrier": "carrier", "planet_teeth": 24}
    ],
}


def make_train(doc, cases):
    full = dict(doc)
    full["cases"] = cases
    return load_document(full)


class PlanetaryTests(unittest.TestCase):
    def test_carrier_fixed(self):
        train = make_train(PLANETARY, [{"name": "c", "inputs": [
            {"axis": "sun", "rpm": 300}, {"axis": "carrier", "fixed": True}]}])
        res = solve(train.constraints + train.cases[0].inputs, train.axes)
        sol = dict(zip(train.axes, res["solution"]))
        self.assertAlmostEqual(sol["carrier"], 0.0)
        self.assertAlmostEqual(sol["ring"], -300 * 24 / 72)

    def test_ring_fixed(self):
        train = make_train(PLANETARY, [{"name": "c", "inputs": [
            {"axis": "sun", "rpm": 480}, {"axis": "ring", "fixed": True}]}])
        res = solve(train.constraints + train.cases[0].inputs, train.axes)
        sol = dict(zip(train.axes, res["solution"]))
        self.assertAlmostEqual(sol["carrier"], 480 * 24 / 96)
        self.assertAlmostEqual(sol["ring"], 0.0)

    def test_sun_fixed(self):
        train = make_train(PLANETARY, [{"name": "c", "inputs": [
            {"axis": "sun", "fixed": True}, {"axis": "ring", "rpm": 120}]}])
        res = solve(train.constraints + train.cases[0].inputs, train.axes)
        sol = dict(zip(train.axes, res["solution"]))
        self.assertAlmostEqual(sol["carrier"], 120 * 72 / 96)

    def test_bad_geometry_rejected(self):
        doc = json.loads(json.dumps(PLANETARY))
        doc["connections"][0]["planet_teeth"] = 20
        doc["cases"] = [{"name": "c", "inputs": []}]
        with self.assertRaises(InputError):
            load_document(doc)


class MeshTests(unittest.TestCase):
    DOC = {
        "axes": ["a", "b", "c"],
        "connections": [
            {"id": "m1", "type": "external_mesh",
             "gear_a": {"axis": "a", "teeth": 20},
             "gear_b": {"axis": "b", "teeth": 40}},
            {"id": "m2", "type": "internal_mesh",
             "gear_a": {"axis": "b", "teeth": 60},
             "gear_b": {"axis": "c", "teeth": 20}},
        ],
    }

    def test_external_and_internal(self):
        train = make_train(self.DOC, [{"name": "c", "inputs": [
            {"axis": "a", "rpm": 100}]}])
        res = solve(train.constraints + train.cases[0].inputs, train.axes)
        sol = dict(zip(train.axes, res["solution"]))
        self.assertAlmostEqual(sol["b"], -50.0)   # external: opposite
        self.assertAlmostEqual(sol["c"], -150.0)  # internal: same as b

    def test_rigid_connection(self):
        doc = {"axes": ["x", "y"],
               "connections": [{"id": "r", "type": "rigid", "axes": ["x", "y"]}]}
        train = make_train(doc, [{"name": "c", "inputs": [{"axis": "x", "rpm": 7}]}])
        res = solve(train.constraints + train.cases[0].inputs, train.axes)
        self.assertEqual(res["status"], "unique")
        self.assertEqual(dict(zip(train.axes, res["solution"]))["y"], 7.0)

    def test_connection_order_irrelevant(self):
        doc1 = json.loads(json.dumps(self.DOC))
        doc2 = json.loads(json.dumps(self.DOC))
        doc2["connections"].reverse()
        sols = []
        for doc in (doc1, doc2):
            train = make_train(doc, [{"name": "c", "inputs": [
                {"axis": "c", "rpm": 30}]}])
            res = solve(train.constraints + train.cases[0].inputs, train.axes)
            sols.append(res["solution"])
        for x, y in zip(*sols):
            self.assertAlmostEqual(x, y)


class ConflictAndDofTests(unittest.TestCase):
    def test_conflicting_inputs_report_sources(self):
        doc = {"axes": ["a", "b"],
               "connections": [{"id": "m", "type": "external_mesh",
                                "gear_a": {"axis": "a", "teeth": 20},
                                "gear_b": {"axis": "b", "teeth": 20}}]}
        train = make_train(doc, [{"name": "bad", "inputs": [
            {"axis": "a", "rpm": 100}, {"axis": "b", "rpm": 100}]}])
        _, diag = run_case(train, train.cases[0])
        self.assertEqual(diag["status"], "conflicting")
        self.assertIn("m", diag["conflict_sources"])
        self.assertIsNone(diag["power"])

    def test_consistent_duplicate_allowed(self):
        doc = {"axes": ["a", "b"],
               "connections": [{"id": "m", "type": "external_mesh",
                                "gear_a": {"axis": "a", "teeth": 20},
                                "gear_b": {"axis": "b", "teeth": 20}}]}
        train = make_train(doc, [{"name": "ok", "inputs": [
            {"axis": "a", "rpm": 100}, {"axis": "b", "rpm": -100}]}])
        _, diag = run_case(train, train.cases[0])
        self.assertEqual(diag["status"], "unique")

    def test_underdetermined_reports_dof(self):
        doc = {"axes": ["a", "b", "c"],
               "connections": [{"id": "m", "type": "external_mesh",
                                "gear_a": {"axis": "a", "teeth": 20},
                                "gear_b": {"axis": "b", "teeth": 20}}]}
        train = make_train(doc, [{"name": "u", "inputs": [{"axis": "a", "rpm": 10}]}])
        speeds, diag = run_case(train, train.cases[0])
        self.assertEqual(diag["status"], "underdetermined")
        self.assertEqual(diag["dof"], 1)
        self.assertEqual(diag["free_axes"], ["c"])
        self.assertEqual(speeds["a"], 10.0)
        self.assertEqual(speeds["b"], -10.0)
        self.assertIsNone(speeds["c"])  # never forced to zero
        self.assertIsNone(diag["power"])


class PowerTests(unittest.TestCase):
    def test_power_balance_and_units(self):
        doc = {"axes": ["in", "out"],
               "connections": [{"id": "m", "type": "external_mesh",
                                "gear_a": {"axis": "in", "teeth": 20},
                                "gear_b": {"axis": "out", "teeth": 40}}]}
        case = {"name": "p", "inputs": [{"axis": "in", "rpm": 100}],
                "torques": {"in": 10.0, "out": 20.0}}
        train = make_train(doc, [case])
        _, diag = run_case(train, train.cases[0])
        self.assertEqual(diag["status"], "unique")
        p_in = 10.0 * 100 * RPM_TO_RAD_S
        p_out = 20.0 * -50 * RPM_TO_RAD_S
        self.assertAlmostEqual(diag["power"]["per_axis"]["in"]["power_W"], p_in)
        self.assertAlmostEqual(diag["power"]["per_axis"]["out"]["power_W"], p_out)
        self.assertAlmostEqual(diag["power"]["total_W"], p_in + p_out)
        self.assertTrue(diag["power"]["balanced"])

    def test_missing_torque_defaults_zero(self):
        doc = {"axes": ["a"], "connections": []}
        train = make_train(doc, [{"name": "z", "inputs": [{"axis": "a", "rpm": 5}]}])
        _, diag = run_case(train, train.cases[0])
        self.assertEqual(diag["power"]["total_W"], 0.0)


class CliTests(unittest.TestCase):
    def test_end_to_end(self):
        doc = dict(PLANETARY)
        doc["cases"] = [
            {"name": "lock_ring", "inputs": [
                {"axis": "sun", "rpm": 480}, {"axis": "ring", "fixed": True}],
             "torques": {"sun": 5.0}},
            {"name": "clash", "inputs": [
                {"axis": "sun", "rpm": 1}, {"axis": "ring", "rpm": 1},
                {"axis": "carrier", "rpm": 1}]},
        ]
        with tempfile.TemporaryDirectory() as tmp:
            src = os.path.join(tmp, "in.json")
            with open(src, "w", encoding="utf-8") as fh:
                json.dump(doc, fh)
            proc = subprocess.run(
                [sys.executable, "-m", "gear_kinematics", src, "-o", tmp],
                capture_output=True, text=True)
            self.assertEqual(proc.returncode, 0, proc.stderr)
            with open(os.path.join(tmp, "speeds.csv"), encoding="utf-8") as fh:
                csv_text = fh.read()
            self.assertIn("lock_ring,carrier,120", csv_text)
            with open(os.path.join(tmp, "diagnostics.json"), encoding="utf-8") as fh:
                diag = json.load(fh)
            clash = [c for c in diag["cases"] if c["case"] == "clash"][0]
            self.assertEqual(clash["status"], "unique")  # whole train rotates together
            lock = [c for c in diag["cases"] if c["case"] == "lock_ring"][0]
            self.assertLess(lock["max_abs_residual"], 1e-6)


if __name__ == "__main__":
    unittest.main()

    def test_non_integer_teeth_rejected(self):
        doc = json.loads(json.dumps(PLANETARY))
        doc["connections"][0]["sun"]["teeth"] = 24.5
        doc["cases"] = [{"name": "c", "inputs": []}]
        with self.assertRaises(InputError):
            load_document(doc)
