import math
import unittest

from battery_pulse.model import ModelError, load_battery
from battery_pulse.sim import Segment, simulate


def make_battery():
    return load_battery({
        "capacity_ah": 1.0,
        "r0_ohm": 0.1,
        "ocv_grid": [
            {"soc": 0.0, "voltage": 3.0},
            {"soc": 0.5, "voltage": 3.6},
            {"soc": 1.0, "voltage": 4.0},
        ],
        "rc_branches": [{"r_ohm": 0.05, "c_f": 1000.0}],
    })


class TestModelValidation(unittest.TestCase):
    def test_decreasing_ocv_rejected(self):
        with self.assertRaises(ModelError):
            load_battery({
                "capacity_ah": 1.0, "r0_ohm": 0.1,
                "ocv_grid": [{"soc": 0.0, "voltage": 4.0},
                             {"soc": 1.0, "voltage": 3.0}],
            })

    def test_bad_capacity_rejected(self):
        with self.assertRaises(ModelError):
            load_battery({
                "capacity_ah": 0.0, "r0_ohm": 0.1,
                "ocv_grid": [{"soc": 0.0, "voltage": 3.0},
                             {"soc": 1.0, "voltage": 4.0}],
            })

    def test_grid_must_span_zero_to_one(self):
        with self.assertRaises(ModelError):
            load_battery({
                "capacity_ah": 1.0, "r0_ohm": 0.1,
                "ocv_grid": [{"soc": 0.1, "voltage": 3.0},
                             {"soc": 1.0, "voltage": 4.0}],
            })


class TestRestRecovery(unittest.TestCase):
    def test_polarization_decays_during_rest(self):
        bat = make_battery()
        segs = [Segment(100.0, 2.0), Segment(500.0, 0.0)]
        samples, report = simulate(bat, segs, v_min=1.0, soc0=1.0)
        self.assertEqual(report["stop_reason"], "completed")
        rest = [s for s in samples if s.t > 100.0 and s.current_a == 0.0]
        self.assertTrue(rest)
        self.assertGreater(rest[-1].v_term, rest[0].v_term)
        expected = rest[0].v_pol * math.exp(-(rest[-1].t - rest[0].t) / 50.0)
        self.assertAlmostEqual(rest[-1].v_pol, expected, places=9)
        self.assertAlmostEqual(rest[-1].soc, rest[0].soc, places=12)


class TestJumpCutoff(unittest.TestCase):
    def test_current_step_triggers_immediate_cutoff(self):
        bat = make_battery()
        segs = [Segment(10.0, 0.0), Segment(10.0, 20.0)]
        samples, report = simulate(bat, segs, v_min=2.5, soc0=1.0)
        self.assertEqual(report["stop_reason"], "voltage_cutoff")
        self.assertAlmostEqual(report["stop_time_s"], 10.0, places=9)
        self.assertAlmostEqual(report["net_discharge_ah"], 0.0, places=12)
        at_t = [s for s in samples if abs(s.t - 10.0) < 1e-9]
        sides = {s.side for s in at_t}
        self.assertIn("left", sides)
        self.assertIn("right", sides)
        right = next(s for s in at_t if s.side == "right")
        self.assertAlmostEqual(right.v_term, 2.0, places=9)
        right = next(s for s in at_t if s.side == "right")
        self.assertAlmostEqual(right.v_term, 2.0, places=9)


class TestInSegmentCutoff(unittest.TestCase):
    def test_voltage_crossing_inside_segment(self):
        bat = make_battery()
        segs = [Segment(3600.0, 1.0)]
        samples, report = simulate(bat, segs, v_min=3.2, soc0=1.0)
        self.assertEqual(report["stop_reason"], "voltage_cutoff")
        t_stop = report["stop_time_s"]
        self.assertGreater(t_stop, 0.0)
        self.assertLess(t_stop, 3600.0)
        last = samples[-1]
        self.assertAlmostEqual(last.v_term, 3.2, places=6)
        self.assertAlmostEqual(last.t, t_stop, places=9)

    def test_stop_time_independent_of_sample_density(self):
        bat = make_battery()
        segs = [Segment(3600.0, 1.0)]
        _, coarse = simulate(bat, segs, v_min=3.2, soc0=1.0, sample_dt=10.0)
        _, fine = simulate(bat, segs, v_min=3.2, soc0=1.0, sample_dt=0.05)
        self.assertAlmostEqual(coarse["stop_time_s"], fine["stop_time_s"],
                               places=7)

    def test_soc_depletion_stops_at_zero(self):
        bat = make_battery()
        segs = [Segment(1e9, 1.0)]
        _, report = simulate(bat, segs, v_min=0.0, soc0=0.5)
        self.assertEqual(report["stop_reason"], "soc_depleted")
        self.assertAlmostEqual(report["stop_time_s"], 1800.0, places=6)
        self.assertAlmostEqual(report["final_soc"], 0.0, places=12)
        self.assertAlmostEqual(report["net_discharge_ah"], 0.5, places=9)


class TestInitialConditions(unittest.TestCase):
    def test_initial_polarization_applied(self):
        bat = make_battery()
        samples, _ = simulate(bat, [Segment(1.0, 0.0)], v_min=0.0,
                              soc0=0.8, v_pol0=[0.3])
        self.assertAlmostEqual(samples[0].v_pol, 0.3, places=12)
        self.assertAlmostEqual(samples[0].v_term, bat.ocv(0.8) - 0.3, places=9)

    def test_invalid_initial_soc_rejected(self):
        bat = make_battery()
        with self.assertRaises(ModelError):
            simulate(bat, [Segment(1.0, 0.0)], v_min=0.0, soc0=1.5)


if __name__ == "__main__":
    unittest.main()
