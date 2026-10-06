import math
import unittest

from spin_echo.config import load_experiment
from spin_echo.simulate import _apply_pulse, _free_evolve, run


def make_exp(**overrides):
    data = {
        "spins": [
            {
                "weight": 1.0,
                "initial": [0.0, 1.0, 0.0],
                "offset_hz": 0.0,
                "t1": "inf",
                "t2": "inf",
            }
        ],
        "pulses": [],
        "samples": {"start": 0.0, "stop": 1.0, "step": 0.1},
    }
    data.update(overrides)
    return load_experiment(data)


class FreeEvolutionTests(unittest.TestCase):
    def test_free_decay_exact(self):
        state = _free_evolve((0.0, 1.0, 0.0), 2.0, math.inf, 0.5, 1.0, 0.25)
        expected = math.exp(-0.25 / 0.5)
        self.assertAlmostEqual(state[0], 0.0, places=12)
        self.assertAlmostEqual(state[1], -expected, places=12)
        half = _free_evolve((0.0, 1.0, 0.0), 2.0, math.inf, 0.5, 1.0, 0.125)
        half = _free_evolve(half, 2.0, math.inf, 0.5, 1.0, 0.125)
        for a, b in zip(state, half):
            self.assertAlmostEqual(a, b, places=12)

    def test_right_hand_rotation_about_z(self):
        mx, my, _ = _free_evolve((1.0, 0.0, 0.0), 0.25, math.inf, math.inf, 1.0, 1.0)
        self.assertAlmostEqual(mx, 0.0, places=12)
        self.assertAlmostEqual(my, 1.0, places=12)

    def test_t1_recovery_toward_equilibrium(self):
        _, _, mz = _free_evolve((0.0, 0.0, 0.0), 0.0, 2.0, math.inf, 1.0, 1.0)
        self.assertAlmostEqual(mz, 1.0 - math.exp(-0.5), places=12)

    def test_infinite_relaxation_keeps_magnitudes(self):
        mx, my, mz = _free_evolve((0.6, 0.8, 0.3), 1.5, math.inf, math.inf, 1.0, 0.7)
        self.assertAlmostEqual(math.hypot(mx, my), 1.0, places=12)
        self.assertAlmostEqual(mz, 0.3, places=12)


class PulseTests(unittest.TestCase):
    def test_180_about_x_flips_y_and_z(self):
        mx, my, mz = _apply_pulse((0.1, 0.9, 0.4), 0.0, math.pi)
        self.assertAlmostEqual(mx, 0.1, places=12)
        self.assertAlmostEqual(my, -0.9, places=12)
        self.assertAlmostEqual(mz, -0.4, places=12)

    def test_90_about_x_takes_z_to_y(self):
        mx, my, mz = _apply_pulse((0.0, 0.0, 1.0), 0.0, math.pi / 2)
        self.assertAlmostEqual(my, 1.0, places=12)
        self.assertAlmostEqual(mz, 0.0, places=12)

    def test_same_time_pulse_order_matters(self):
        base = {
            "spins": [{"weight": 1.0, "initial": [0.0, 0.0, 1.0],
                        "t1": "inf", "t2": "inf"}],
            "samples": [0.0],
        }
        pa = {"time": 0.0, "phase_deg": 0.0, "flip_deg": 90.0}
        pb = {"time": 0.0, "phase_deg": 90.0, "flip_deg": 180.0}
        exp_ab = load_experiment({**base, "pulses": [pa, pb]})
        exp_ba = load_experiment({**base, "pulses": [pb, pa]})
        state_ab = run(exp_ab)[0][1][0]
        state_ba = run(exp_ba)[0][1][0]
        self.assertNotAlmostEqual(state_ab[1], state_ba[1], places=6)
        s = _apply_pulse((0.0, 0.0, 1.0), 0.0, math.pi / 2)
        s = _apply_pulse(s, math.pi / 2, math.pi)
        for a, b in zip(state_ab, s):
            self.assertAlmostEqual(a, b, places=12)

    def test_sample_after_all_pulses_at_same_time(self):
        exp = load_experiment({
            "spins": [{"weight": 1.0, "initial": [0.0, 0.0, 1.0],
                        "t1": "inf", "t2": "inf"}],
            "pulses": [
                {"time": 0.5, "phase_deg": 0.0, "flip_deg": 90.0},
                {"time": 0.5, "phase_deg": 0.0, "flip_deg": 90.0},
            ],
            "samples": [0.5],
        })
        _, states = run(exp)[0]
        self.assertAlmostEqual(states[0][2], -1.0, places=12)
