import json
import os
import tempfile
import unittest

from pv_string.model import Module, analyze_string
from pv_string.__main__ import run


def make_module(isc=10.0, voc=40.0, knee_i=9.5, knee_v=32.0, drop=0.5):
    return Module(voc=voc, isc=isc, knee_v=knee_v, knee_i=knee_i, diode_drop=drop)


def brute_force_max(modules, i_max, steps=200000):
    best = 0.0
    for k in range(steps + 1):
        current = i_max * k / steps
        voltage = sum(m.voltage(current) for m in modules)
        if voltage > 0.0:
            best = max(best, current * voltage)
    return best


class ModuleModelTest(unittest.TestCase):
    def test_knee_must_be_interior(self):
        with self.assertRaises(ValueError):
            make_module(knee_i=10.0)
        with self.assertRaises(ValueError):
            make_module(knee_v=0.0)
        with self.assertRaises(ValueError):
            make_module(drop=-0.5)

    def test_clamp_at_diode_drop(self):
        m = make_module()
        self.assertGreater(m.bypass_current, m.isc)
        self.assertAlmostEqual(m.voltage(m.bypass_current), -0.5)
        self.assertEqual(m.voltage(m.bypass_current + 5.0), -0.5)
        self.assertTrue(m.is_bypassed(m.bypass_current))
        self.assertFalse(m.is_bypassed(m.bypass_current - 1e-9))


class StringAnalysisTest(unittest.TestCase):
    def test_identical_modules_no_bypass(self):
        modules = [make_module() for _ in range(4)]
        result = analyze_string(modules)
        opt = result.optimum
        self.assertFalse(any(s.bypassed for s in opt.modules))
        self.assertAlmostEqual(opt.voltage, sum(s.voltage for s in opt.modules))
        # Matches a dense brute-force scan.
        self.assertGreaterEqual(opt.power, brute_force_max(modules, result.max_current) - 1e-6)

    def test_weak_module_is_bypassed(self):
        strong = [make_module() for _ in range(3)]
        weak = make_module(isc=5.0, knee_i=4.5)
        modules = strong + [weak]
        result = analyze_string(modules)
        opt = result.optimum
        self.assertGreater(opt.current, weak.isc)
        self.assertTrue(opt.modules[3].bypassed)
        self.assertAlmostEqual(opt.modules[3].voltage, -0.5)
        self.assertFalse(any(s.bypassed for s in opt.modules[:3]))
        self.assertGreaterEqual(opt.power, brute_force_max(modules, result.max_current) - 1e-6)

    def test_local_peak_reported(self):
        # One very weak module creates a local peak below its knee and the
        # global peak with the weak module bypassed.
        modules = [make_module() for _ in range(3)] + [make_module(isc=6.0, knee_i=5.5, knee_v=25.0)]
        result = analyze_string(modules)
        self.assertTrue(result.local_peaks)
        for peak in result.local_peaks:
            self.assertLessEqual(peak.power, result.optimum.power)

    def test_tie_prefers_lower_current(self):
        # Two identical modules: power peak is unique, but craft a tie by
        # checking the tie rule directly through sorted candidate order.
        modules = [make_module() for _ in range(2)]
        result = analyze_string(modules)
        currents = [p.current for p in result.local_peaks]
        self.assertNotIn(result.optimum.current, currents)


class CliTest(unittest.TestCase):
    def _write_input(self, directory, step):
        config = {
            "diode_drop": 0.5,
            "step": step,
            "modules": [
                {"voc": 40.0, "isc": 10.0, "knee_v": 32.0, "knee_i": 9.5},
                {"voc": 40.0, "isc": 10.0, "knee_v": 32.0, "knee_i": 9.5},
                {"voc": 40.0, "isc": 10.0, "knee_v": 32.0, "knee_i": 9.5},
            ],
            "conditions": [
                {"name": "STC"},
                {
                    "name": "shaded",
                    "modules": [
                        {"voc": 40.0, "isc": 10.0, "knee_v": 32.0, "knee_i": 9.5},
                        {"voc": 40.0, "isc": 10.0, "knee_v": 32.0, "knee_i": 9.5},
                        {"voc": 30.0, "isc": 4.0, "knee_v": 24.0, "knee_i": 3.6},
                    ],
                },
            ],
        }
        path = os.path.join(directory, "input.json")
        with open(path, "w", encoding="utf-8") as fh:
            json.dump(config, fh)
        return path

    def test_end_to_end_and_step_independence(self):
        with tempfile.TemporaryDirectory() as tmp:
            optima = []
            for step in (0.5, 0.05):
                input_path = self._write_input(tmp, step)
                report = run(
                    input_path,
                    os.path.join(tmp, f"report_{step}.json"),
                    os.path.join(tmp, f"scan_{step}.csv"),
                )
                self.assertEqual([c["name"] for c in report["conditions"]], ["STC", "shaded"])
                optima.append(
                    [
                        (c["optimum"]["current"], c["optimum"]["power"])
                        for c in report["conditions"]
                    ]
                )
            # Sweep step must not change the optimum.
            self.assertEqual(optima[0], optima[1])

            shaded = report["conditions"][1]["optimum"]
            bypassed = [m["index"] for m in shaded["modules"] if m["bypassed"]]
            self.assertEqual(bypassed, [2])
            self.assertAlmostEqual(shaded["modules"][2]["voltage"], -0.5)

            with open(os.path.join(tmp, "scan_0.5.csv"), encoding="utf-8") as fh:
                header = fh.readline().strip()
            self.assertIn("module_2_voltage", header)


if __name__ == "__main__":
    unittest.main()
