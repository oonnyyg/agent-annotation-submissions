import unittest

from pv_string.model import Module
from pv_string.solver import find_mpp, string_voltage


def uniform_modules(n=3):
    return [Module(voc=40.0, isc=10.0, vknee=32.0, iknee=9.5) for _ in range(n)]


class ModelTest(unittest.TestCase):
    def test_validation(self):
        with self.assertRaises(ValueError):
            Module(voc=40, isc=10, vknee=45, iknee=9)
        with self.assertRaises(ValueError):
            Module(voc=40, isc=10, vknee=32, iknee=10)
        with self.assertRaises(ValueError):
            Module(voc=40, isc=10, vknee=32, iknee=9, diode_drop=0)

    def test_endpoints_and_clamp(self):
        m = Module(voc=40.0, isc=10.0, vknee=32.0, iknee=9.5, diode_drop=0.5)
        self.assertAlmostEqual(m.voltage(0.0), 40.0)
        self.assertAlmostEqual(m.voltage(9.5), 32.0)
        self.assertAlmostEqual(m.voltage(10.0), 0.0)
        self.assertEqual(m.voltage(100.0), -0.5)
        self.assertTrue(m.bypassed(100.0))
        self.assertFalse(m.bypassed(5.0))


class MppTest(unittest.TestCase):
    def test_identical_modules(self):
        modules = uniform_modules(3)
        peak = find_mpp(modules)
        m = modules[0]
        # Both segment vertices fall outside their segments here, so the
        # MPP sits exactly at the knee point.
        i_star = m.iknee
        expected_p = 3 * i_star * m.voltage(i_star)
        self.assertAlmostEqual(peak.current, i_star, places=7)
        self.assertAlmostEqual(peak.power, expected_p, places=6)
        self.assertFalse(any(peak.bypassed))

    def test_weak_module_bypassed(self):
        strong = uniform_modules(2)
        weak = Module(voc=40.0, isc=4.0, vknee=32.0, iknee=3.8, diode_drop=0.5)
        modules = strong + [weak]
        peak = find_mpp(modules)
        self.assertGreater(peak.current, weak.isc)
        self.assertTrue(peak.bypassed[2])
        self.assertAlmostEqual(peak.module_voltages[2], -0.5)
        self.assertFalse(peak.bypassed[0])

    def test_local_peak_not_mistaken(self):
        modules = uniform_modules(2) + [
            Module(voc=40.0, isc=2.0, vknee=32.0, iknee=1.9, diode_drop=0.5)
        ]
        peak = find_mpp(modules)
        self.assertTrue(peak.bypassed[2])
        local_i = 1.9
        local_p = local_i * string_voltage(modules, local_i)
        self.assertGreater(peak.power, local_p)

    def test_no_power_beyond_zero_voltage(self):
        modules = uniform_modules(1)
        peak = find_mpp(modules)
        self.assertGreaterEqual(peak.voltage, 0.0)
        self.assertGreaterEqual(peak.current, 0.0)


if __name__ == "__main__":
    unittest.main()
