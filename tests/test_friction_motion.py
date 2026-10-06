import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from friction_motion.core import scenario_from_dict, simulate, ValidationError
from friction_motion.sampling import sample_rows


def make_scenario(**overrides):
    base = {
        "name": "t",
        "mass": 2.0,
        "gravity": 10.0,
        "mu_static": 0.5,
        "mu_kinetic": 0.4,
        "x0": 0.0,
        "v0": 0.0,
        "t_end": 4.0,
        "dt": 0.1,
        "forces": [{"t_start": 0.0, "t_end": 4.0, "value": 0.0}],
    }
    base.update(overrides)
    return scenario_from_dict(base)


class TestValidation(unittest.TestCase):
    def test_static_must_be_at_least_kinetic(self):
        with self.assertRaises(ValidationError):
            make_scenario(mu_static=0.3, mu_kinetic=0.4)

    def test_negative_coefficients_rejected(self):
        with self.assertRaises(ValidationError):
            make_scenario(mu_kinetic=-0.1)

    def test_forces_must_start_at_zero_and_cover_range(self):
        with self.assertRaises(ValidationError):
            make_scenario(forces=[{"t_start": 0.5, "t_end": 4.0, "value": 1.0}])
        with self.assertRaises(ValidationError):
            make_scenario(forces=[{"t_start": 0.0, "t_end": 3.0, "value": 1.0}])
        with self.assertRaises(ValidationError):
            make_scenario(forces=[
                {"t_start": 0.0, "t_end": 2.0, "value": 1.0},
                {"t_start": 2.5, "t_end": 4.0, "value": 1.0},
            ])


class TestStaticThreshold(unittest.TestCase):
    def test_force_at_static_limit_holds(self):
        # f_s_max = 0.5 * 2 * 10 = 10 N exactly: must remain at rest.
        sc = make_scenario(forces=[{"t_start": 0.0, "t_end": 4.0, "value": 10.0}])
        result = simulate(sc)
        self.assertEqual(result["final"]["x"], 0.0)
        self.assertEqual(result["final"]["v"], 0.0)
        self.assertFalse(any(e["type"] == "start" for e in result["events"]))

    def test_force_just_above_static_limit_starts(self):
        sc = make_scenario(forces=[{"t_start": 0.0, "t_end": 4.0, "value": 10.5}])
        result = simulate(sc)
        self.assertGreater(result["final"]["x"], 0.0)
        self.assertTrue(any(e["type"] == "start" for e in result["events"]))

    def test_force_below_limit_holds_negative_side(self):
        sc = make_scenario(forces=[{"t_start": 0.0, "t_end": 4.0, "value": -9.0}])
        result = simulate(sc)
        self.assertEqual(result["final"]["x"], 0.0)


class TestStopAndReverse(unittest.TestCase):
    def test_stop_then_reverse_after_force_switch(self):
        # Slide forward, brake to a stop, then reverse under new force.
        sc = make_scenario(
            t_end=4.0,
            forces=[
                {"t_start": 0.0, "t_end": 1.0, "value": 28.0},   # a = (28-8)/2 = 10
                {"t_start": 1.0, "t_end": 2.0, "value": 0.0},    # a = -4, stop at 1+10/4... v=10 -> 2.5s? no
                {"t_start": 2.0, "t_end": 4.0, "value": -28.0},
            ],
        )
        # phase1: v(1)=10, x(1)=5. phase2: a=-4, at t=2 v=6, x=13.
        # phase3: F=-28, a=(-28-8)/2=-18, stop at t=2+6/18, then reverse
        # with a=(-28+8)/2=-10.
        result = simulate(sc)
        stops = [e for e in result["events"] if e["type"] == "stop"]
        starts = [e for e in result["events"] if e["type"] == "start"]
        self.assertEqual(len(stops), 1)
        self.assertAlmostEqual(stops[0]["t"], 2.0 + 6.0 / 18.0)
        reverse = [e for e in starts if e.get("direction") == -1]
        self.assertEqual(len(reverse), 1)
        self.assertEqual(reverse[0]["t"], stops[0]["t"])
        self.assertLess(result["final"]["v"], 0.0)
        # No spurious displacement at the stop instant: position is
        # continuous across the stop/restart events.
        pieces = result["pieces"]
        for prev, nxt in zip(pieces, pieces[1:]):
            x_prev = prev.state_at(prev.t1)[0]
            x_next = nxt.state_at(nxt.t0)[0]
            self.assertAlmostEqual(x_prev, x_next, places=9)

    def test_stop_and_hold_when_force_small(self):
        sc = make_scenario(
            t_end=4.0,
            forces=[
                {"t_start": 0.0, "t_end": 1.0, "value": 28.0},
                {"t_start": 1.0, "t_end": 4.0, "value": 0.0},  # < f_s_max = 10
            ],
        )
        result = simulate(sc)
        stops = [e for e in result["events"] if e["type"] == "stop"]
        self.assertEqual(len(stops), 1)
        self.assertAlmostEqual(stops[0]["t"], 1.0 + 10.0 / 4.0)
        self.assertEqual(result["final"]["v"], 0.0)
        x_stop = 5.0 + (10.0 * 2.5 - 2.0 * 2.5 * 2.5)
        self.assertAlmostEqual(result["final"]["x"], x_stop)


class TestEnergyConsistency(unittest.TestCase):
    def check_energy(self, sc):
        result = simulate(sc)
        self.assertAlmostEqual(result["energy"]["residual"], 0.0, places=9)
        return result

    def test_residual_zero_for_mixed_motion(self):
        sc = make_scenario(
            t_end=5.0,
            forces=[
                {"t_start": 0.0, "t_end": 1.0, "value": 28.0},
                {"t_start": 1.0, "t_end": 2.5, "value": -40.0},
                {"t_start": 2.5, "t_end": 4.0, "value": 3.0},
                {"t_start": 4.0, "t_end": 5.0, "value": -28.0},
            ],
        )
        result = self.check_energy(sc)
        self.assertGreater(result["distance"], abs(result["net_displacement"]))

    def test_results_independent_of_sampling_dt(self):
        kwargs = dict(t_end=3.0, forces=[
            {"t_start": 0.0, "t_end": 1.0, "value": 28.0},
            {"t_start": 1.0, "t_end": 3.0, "value": -28.0},
        ])
        r1 = simulate(make_scenario(dt=0.1, **kwargs))
        r2 = simulate(make_scenario(dt=0.01, **kwargs))
        self.assertEqual(r1["energy"], r2["energy"])
        self.assertEqual(r1["distance"], r2["distance"])
        self.assertEqual(r1["events"], r2["events"])

    def test_sampling_covers_range_and_matches_analytic(self):
        sc = make_scenario(t_end=1.0, dt=0.3,
                           forces=[{"t_start": 0.0, "t_end": 1.0, "value": 28.0}])
        result = simulate(sc)
        rows = list(sample_rows(result["pieces"], sc.t_end, sc.dt))
        self.assertEqual(rows[0][0], 0.0)
        self.assertAlmostEqual(rows[-1][0], 1.0)
        # a = (28-8)/2 = 10 -> x(1) = 5, v(1) = 10
        self.assertAlmostEqual(rows[-1][1], 5.0)
        self.assertAlmostEqual(rows[-1][2], 10.0)


class TestCliBatch(unittest.TestCase):
    def test_batch_with_failing_scenario(self):
        good = {
            "name": "good", "mass": 1.0, "gravity": 10.0,
            "mu_static": 0.5, "mu_kinetic": 0.4,
            "x0": 0.0, "v0": 0.0, "t_end": 1.0, "dt": 0.5,
            "forces": [{"t_start": 0.0, "t_end": 1.0, "value": 20.0}],
        }
        bad = dict(good, name="bad", mu_static=0.1)  # mu_s < mu_k
        with tempfile.TemporaryDirectory() as tmp:
            src = Path(tmp) / "in.json"
            src.write_text(json.dumps({"scenarios": [good, bad]}),
                           encoding="utf-8")
            proc = subprocess.run(
                [sys.executable, "-m", "friction_motion", str(src),
                 "-o", tmp],
                capture_output=True, text=True,
                cwd=Path(__file__).resolve().parent.parent)
            self.assertEqual(proc.returncode, 1)
            results = json.loads((Path(tmp) / "results.json").read_text())
            self.assertEqual(results[0]["status"], "ok")
            self.assertEqual(results[1]["status"], "error")
            self.assertIn("mu_static", results[1]["reason"])
            self.assertTrue((Path(tmp) / "good_trajectory.csv").exists())
            self.assertFalse((Path(tmp) / "bad_trajectory.csv").exists())


if __name__ == "__main__":
    unittest.main()
