import json
import math
import unittest

from spin_echo.config import ConfigError, load_config
from spin_echo.physics import apply_pulse, free_evolve
from spin_echo.simulation import simulate, summarize


def make_config(**overrides):
    base = {
        "spins": [
            {"name": "a", "weight": 1.0, "m0": [1.0, 0.0, 0.0],
             "offset_hz": 0.0, "t1": None, "t2": None, "mz_eq": 0.0}
        ],
        "pulses": [],
        "samples": [0.0],
    }
    base.update(overrides)
    return load_config(json.dumps(base))


class TestFreeEvolution(unittest.TestCase):
    def test_t2_decay_and_rotation(self):
        m = free_evolve((1.0, 0.0, 0.0), 1.0, math.inf, 2.0, 0.0, 0.25)
        self.assertAlmostEqual(m[0], 0.0, places=12)
        self.assertAlmostEqual(m[1], math.exp(-0.125), places=12)

    def test_t1_recovery_toward_equilibrium(self):
        m = free_evolve((0.0, 0.0, 0.0), 0.0, 1.0, math.inf, 1.0, 1.0)
        self.assertAlmostEqual(m[2], 1.0 - math.exp(-1.0), places=12)

    def test_infinite_relaxation_no_decay(self):
        m = free_evolve((0.5, -0.25, 0.3), 0.0, math.inf, math.inf, 9.0, 10.0)
        self.assertEqual(m, (0.5, -0.25, 0.3))

    def test_interval_splitting_is_exact(self):
        m1 = free_evolve((0.7, 0.1, 0.2), 3.3, 0.8, 0.5, 0.4, 1.1)
        mh = free_evolve((0.7, 0.1, 0.2), 3.3, 0.8, 0.5, 0.4, 0.55)
        m2 = free_evolve(mh, 3.3, 0.8, 0.5, 0.4, 0.55)
        for a, b in zip(m1, m2):
            self.assertAlmostEqual(a, b, places=12)


class TestPulses(unittest.TestCase):
    def test_pi_over_2_about_x_tips_z(self):
        m = apply_pulse((0.0, 0.0, 1.0), 0.0, math.pi / 2)
        self.assertAlmostEqual(m[0], 0.0, places=12)
        self.assertAlmostEqual(abs(m[1]), 1.0, places=12)
        self.assertAlmostEqual(m[2], 0.0, places=12)

    def test_pi_about_y_flips_x_sign(self):
        m = apply_pulse((1.0, 0.0, 0.0), math.pi / 2, math.pi)
        self.assertAlmostEqual(m[0], -1.0, places=12)
        self.assertAlmostEqual(m[1], 0.0, places=12)

    def test_same_time_pulse_order_matters(self):
        def seq(pulses):
            cfg = make_config(
                spins=[{"name": "a", "weight": 1.0, "m0": [0.0, 0.0, 1.0],
                        "offset_hz": 0.0, "t1": None, "t2": None}],
                pulses=[{"time": 0.0, "phase": ph, "flip": fl}
                        for ph, fl in pulses],
                samples=[0.0],
            )
            rec = simulate(cfg)[0]
            return rec["groups"][0][1:]

        a = seq([(0.0, math.pi / 2), (math.pi / 2, math.pi / 2)])
        b = seq([(math.pi / 2, math.pi / 2), (0.0, math.pi / 2)])
        self.assertFalse(all(abs(x - y) < 1e-9 for x, y in zip(a, b)))
        self.assertAlmostEqual(a[0], 0.0, places=12)
        self.assertAlmostEqual(a[1], -1.0, places=12)
        self.assertAlmostEqual(a[2], 0.0, places=12)


class TestSpinEcho(unittest.TestCase):
    def test_detuning_refocuses_at_echo(self):
        tau = 0.01
        cfg = make_config(
            spins=[
                {"name": "up", "weight": 1.0, "m0": [1.0, 0.0, 0.0],
                 "offset_hz": 30.0, "t1": None, "t2": None},
                {"name": "down", "weight": 1.0, "m0": [1.0, 0.0, 0.0],
                 "offset_hz": -30.0, "t1": None, "t2": None},
            ],
            pulses=[{"time": tau, "phase": math.pi / 2, "flip": math.pi}],
            samples=[tau, 1.5 * tau, 2.0 * tau],
        )
        recs = simulate(cfg)
        self.assertAlmostEqual(recs[2]["amplitude"], 1.0, places=9)
        self.assertLess(recs[1]["amplitude"], 1.0)

    def test_t2_decay_not_refocused(self):
        tau = 0.05
        cfg = make_config(
            spins=[{"name": "a", "weight": 1.0, "m0": [1.0, 0.0, 0.0],
                    "offset_hz": 25.0, "t1": None, "t2": 0.1}],
            pulses=[{"time": tau, "phase": 0.0, "flip": math.pi}],
            samples=[2.0 * tau],
        )
        rec = simulate(cfg)[0]
        self.assertAlmostEqual(rec["amplitude"], math.exp(-0.1 / 0.1),
                               places=9)

    def test_zero_amplitude_phase_is_none(self):
        cfg = make_config(
            spins=[
                {"name": "p", "weight": 1.0, "m0": [1.0, 0.0, 0.0],
                 "offset_hz": 0.0, "t1": None, "t2": None},
                {"name": "n", "weight": 1.0, "m0": [-1.0, 0.0, 0.0],
                 "offset_hz": 0.0, "t1": None, "t2": None},
            ],
            samples=[0.0, 0.5],
        )
        for rec in simulate(cfg):
            self.assertEqual(rec["amplitude"], 0.0)
            self.assertIsNone(rec["phase"])

    def test_weight_normalization(self):
        cfg = make_config(
            spins=[
                {"name": "a", "weight": 3.0, "m0": [1.0, 0.0, 0.0],
                 "offset_hz": 0.0, "t1": None, "t2": None},
                {"name": "b", "weight": 1.0, "m0": [0.0, 1.0, 0.0],
                 "offset_hz": 0.0, "t1": None, "t2": None},
            ],
            samples=[0.0],
        )
        rec = simulate(cfg)[0]
        self.assertAlmostEqual(rec["amplitude"],
                               math.hypot(0.75, 0.25), places=12)

    def test_sample_order_does_not_change_evolution(self):
        kwargs = dict(
            spins=[{"name": "a", "weight": 1.0, "m0": [1.0, 0.0, 0.0],
                    "offset_hz": 7.0, "t1": 0.5, "t2": 0.3, "mz_eq": 0.2}],
            pulses=[{"time": 0.02, "phase": 0.3, "flip": 2.1}],
        )
        cfg1 = make_config(samples=[0.01, 0.02, 0.03, 0.05], **kwargs)
        cfg2 = make_config(samples=[0.05, 0.03, 0.01, 0.01, 0.02], **kwargs)
        r1 = {r["time"]: r for r in simulate(cfg1)}
        r2 = {r["time"]: r for r in simulate(cfg2)}
        for t in r1:
            self.assertAlmostEqual(r1[t]["amplitude"], r2[t]["amplitude"],
                                  places=15)

    def test_peak_window_earliest_time(self):
        cfg = make_config(
            spins=[{"name": "a", "weight": 1.0, "m0": [1.0, 0.0, 0.0],
                    "offset_hz": 0.0, "t1": None, "t2": None}],
            samples=[0.0, 0.1, 0.2],
            window=[0.0, 0.2],
        )
        summary = summarize(cfg, simulate(cfg))
        self.assertEqual(summary["peak"]["time"], 0.0)
        self.assertAlmostEqual(summary["peak"]["amplitude"], 1.0)


class TestConfigValidation(unittest.TestCase):
    def expect_error(self, cfg, fragment):
        with self.assertRaises(ConfigError) as ctx:
            load_config(json.dumps(cfg))
        self.assertIn(fragment, str(ctx.exception))

    def test_negative_weight_rejected(self):
        self.expect_error(
            {"spins": [{"weight": -1.0, "m0": [1, 0, 0], "offset_hz": 0,
                        "t1": None, "t2": None}], "samples": [0.0]},
            "config.spins[0].weight")

    def test_zero_total_weight_rejected(self):
        self.expect_error(
            {"spins": [{"weight": 0.0, "m0": [1, 0, 0], "offset_hz": 0,
                        "t1": None, "t2": None}], "samples": [0.0]},
            "total weight")

    def test_bad_m0_located(self):
        self.expect_error(
            {"spins": [{"weight": 1.0, "m0": [1, 0], "offset_hz": 0,
                        "t1": None, "t2": None}], "samples": [0.0]},
            "config.spins[0].m0")

    def test_nonpositive_t2_rejected(self):
        self.expect_error(
            {"spins": [{"weight": 1.0, "m0": [1, 0, 0], "offset_hz": 0,
                        "t1": None, "t2": 0.0}], "samples": [0.0]},
            "config.spins[0].t2")

    def test_bad_pulse_time_located(self):
        self.expect_error(
            {"spins": [{"weight": 1.0, "m0": [1, 0, 0], "offset_hz": 0,
                        "t1": None, "t2": None}],
             "pulses": [{"time": -0.1, "flip": 3.14}],
             "samples": [0.0]},
            "config.pulses[0].time")


if __name__ == "__main__":
    unittest.main()
