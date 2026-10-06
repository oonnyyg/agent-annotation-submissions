import math
import unittest

from spin_echo.config import load_experiment
from spin_echo.simulate import run, total_signal
from spin_echo.report import build_summary


def echo_config(offsets, t2="inf", extra_samples=None):
    return {
        "spins": [
            {"weight": 1.0, "initial": [0.0, 1.0, 0.0],
             "offset_hz": f, "t1": "inf", "t2": t2}
            for f in offsets
        ],
        "pulses": [
            {"time": 0.25, "phase_deg": 0.0, "flip_deg": 180.0},
        ],
        "samples": extra_samples or {"start": 0.0, "stop": 0.5, "step": 0.001},
        "peak_window": [0.4, 0.5],
    }


class EchoTests(unittest.TestCase):
    def test_refocusing_at_echo_time(self):
        exp = load_experiment(echo_config([1.0, 2.0, 3.0, 4.0, 5.0]))
        results = run(exp)
        by_time = {round(t, 9): states for t, states in results}
        mag_mid, _ = total_signal(by_time[0.25], exp.spins)
        mag_echo, phase = total_signal(by_time[0.5], exp.spins)
        self.assertLess(mag_mid, 0.25)
        self.assertAlmostEqual(mag_echo, 1.0, places=6)
        self.assertAlmostEqual(phase, -math.pi / 2, places=6)

    def test_echo_peak_summary(self):
        exp = load_experiment(echo_config([-20.0, 5.0, 25.0], t2=2.0))
        results = run(exp)
        summary = build_summary(exp, results)
        self.assertAlmostEqual(summary["peak"]["time_s"], 0.5, places=9)
        self.assertAlmostEqual(
            summary["peak"]["magnitude"], math.exp(-0.5 / 2.0), places=6
        )

    def test_sampling_does_not_change_evolution(self):
        coarse = load_experiment(echo_config([7.0], extra_samples=[0.5]))
        fine = load_experiment(
            echo_config([7.0], extra_samples={"start": 0.0, "stop": 0.5, "step": 0.001})
        )
        state_coarse = run(coarse)[0][1][0]
        state_fine = run(fine)[-1][1][0]
        for a, b in zip(state_coarse, state_fine):
            self.assertAlmostEqual(a, b, places=12)

    def test_zero_amplitude_phase_is_none(self):
        exp = load_experiment({
            "spins": [
                {"weight": 1.0, "initial": [1.0, 0.0, 0.0], "t1": "inf", "t2": "inf"},
                {"weight": 1.0, "initial": [-1.0, 0.0, 0.0], "t1": "inf", "t2": "inf"},
            ],
            "samples": [0.0],
        })
        _, states = run(exp)[0]
        mag, phase = total_signal(states, exp.spins)
        self.assertEqual(mag, 0.0)
        self.assertIsNone(phase)

    def test_no_relaxation_limit_weight_normalized(self):
        exp = load_experiment({
            "spins": [
                {"weight": 3.0, "initial": [0.0, 1.0, 0.0],
                 "offset_hz": 1.0, "t1": "inf", "t2": "inf"},
                {"weight": 1.0, "initial": [0.0, 1.0, 0.0],
                 "offset_hz": 1.0, "t1": "inf", "t2": "inf"},
            ],
            "samples": [0.1, 0.2],
        })
        for _, states in run(exp):
            mag, _ = total_signal(states, exp.spins)
            self.assertAlmostEqual(mag, 1.0, places=12)
