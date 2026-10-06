import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

from friction_motion.model import ScenarioError, parse_scenario
from friction_motion.simulate import (
    energy_summary,
    sample_trajectory,
    simulate,
)

G = 10.0  # round numbers keep expectations exact


def make_scenario(**overrides):
    base = {
        "name": "test",
        "mass": 1.0,
        "gravity": G,
        "mu_static": 0.3,
        "mu_kinetic": 0.2,
        "x0": 0.0,
        "v0": 0.0,
        "t_end": 5.0,
        "sample_dt": 0.1,
        "force_segments": [[0.0, 5.0]],
    }
    base.update(overrides)
    return parse_scenario(base, 0)


class TestValidation(unittest.TestCase):
    def test_static_must_be_at_least_kinetic(self):
        with self.assertRaises(ScenarioError):
            make_scenario(mu_static=0.1, mu_kinetic=0.2)

    def test_negative_coefficient_rejected(self):
        with self.assertRaises(ScenarioError):
            make_scenario(mu_kinetic=-0.1)

    def test_segments_must_start_at_zero(self):
        with self.assertRaises(ScenarioError):
            make_scenario(force_segments=[[0.5, 1.0]])

    def test_segments_must_cover_range(self):
        with self.assertRaises(ScenarioError):
            make_scenario(t_end=1.0, force_segments=[[0.0, 1.0], [2.0, 3.0]])


class TestStaticThreshold(unittest.TestCase):
    def test_force_at_max_static_stays_stuck(self):
        # f_s_max = 0.3 * 1 * 10 = 3 N exactly: must not move.
        scn = make_scenario(force_segments=[[0.0, 3.0]])
        result = simulate(scn)
        rows = sample_trajectory(result)
        self.assertTrue(all(row["x"] == 0.0 for row in rows))
        self.assertTrue(all(row["v"] == 0.0 for row in rows))
        self.assertEqual(result.events[0].kind, "stick")

    def test_force_just_above_max_static_starts(self):
        scn = make_scenario(force_segments=[[0.0, 3.0 + 1e-9]])
        result = simulate(scn)
        self.assertEqual(result.events[0].kind, "start_moving")
        rows = sample_trajectory(result)
        self.assertGreater(rows[-1]["x"], 0.0)

    def test_stuck_then_released_by_switch(self):
        scn = make_scenario(force_segments=[[0.0, 1.0], [2.0, 5.0]])
        result = simulate(scn)
        kinds = [e.kind for e in result.events]
        self.assertEqual(kinds, ["stick", "force_switch", "start_moving"])
        self.assertEqual(result.events[-1].t, 2.0)


class TestStopAndReverse(unittest.TestCase):
    def test_stop_then_reverse(self):
        # Push +5 N for 1 s, then -8 N: block stops, then moves backward.
        scn = make_scenario(
            t_end=6.0,
            force_segments=[[0.0, 5.0], [1.0, -8.0]],
        )
        result = simulate(scn)
        kinds = [e.kind for e in result.events]
        self.assertIn("stop", kinds)
        stop = result.events[kinds.index("stop")]
        restart = result.events[kinds.index("stop") + 1]
        self.assertEqual(restart.kind, "start_moving")
        self.assertEqual(restart.t, stop.t)  # same instant, no fake jump
        self.assertEqual(restart.x, stop.x)
        rows = sample_trajectory(result)
        self.assertLess(rows[-1]["x"], stop.x)  # ends up behind stop point
        self.assertLess(rows[-1]["v"], 0.0)

    def test_stop_then_stick(self):
        # After the push ends the force drops below f_s_max: block sticks.
        scn = make_scenario(
            t_end=6.0,
            force_segments=[[0.0, 5.0], [1.0, -1.0]],
        )
        result = simulate(scn)
        kinds = [e.kind for e in result.events]
        self.assertIn("stick", kinds)
        stick = result.events[kinds.index("stick")]
        rows = sample_trajectory(result)
        after = [r for r in rows if r["t"] > stick.t + 1e-9]
        self.assertTrue(all(abs(r["x"] - stick.x) < 1e-12 for r in after))

    def test_stop_exactly_on_force_switch_uses_new_force(self):
        # f_k = 2 N, m = 1: sliding accel with F=+5 is +3; with F=-8 is -10.
        # From rest: v(1) = 3. Stop time after switch: 1 + 3/10 = 1.3 s.
        # Choose the second switch exactly at t = 1.3 with a weak force so
        # the block must stop and stick under the *new* segment.
        scn = make_scenario(
            t_end=4.0,
            force_segments=[[0.0, 5.0], [1.0, -8.0], [1.3, 1.0]],
        )
        result = simulate(scn)
        kinds = [e.kind for e in result.events]
        self.assertIn("stop", kinds)
        stop = result.events[kinds.index("stop")]
        self.assertAlmostEqual(stop.t, 1.3, places=9)
        self.assertEqual(stop.force, 1.0)  # judged by the new segment
        self.assertEqual(result.events[kinds.index("stop") + 1].kind, "stick")


class TestEnergy(unittest.TestCase):
    def test_work_energy_balance(self):
        scn = make_scenario(
            t_end=6.0,
            force_segments=[[0.0, 5.0], [1.0, -8.0], [3.0, 0.5]],
        )
        result = simulate(scn)
        energy = energy_summary(result)
        self.assertAlmostEqual(energy["residual"], 0.0, places=9)
        self.assertGreater(energy["distance_total"],
                           abs(energy["displacement_net"]))

    def test_analytic_expectations(self):
        # F=5 N, f_k=2 N, m=1 kg: a=3, over 2 s: x=6, v=6.
        scn = make_scenario(t_end=2.0, force_segments=[[0.0, 5.0]])
        result = simulate(scn)
        rows = sample_trajectory(result)
        self.assertAlmostEqual(rows[-1]["x"], 6.0, places=12)
        self.assertAlmostEqual(rows[-1]["v"], 6.0, places=12)
        energy = energy_summary(result)
        self.assertAlmostEqual(energy["work_external"], 30.0, places=12)
        self.assertAlmostEqual(energy["friction_dissipation"], 12.0, places=12)
        self.assertAlmostEqual(energy["kinetic_energy_change"], 18.0, places=12)

    def test_sampling_frequency_does_not_change_results(self):
        kwargs = dict(t_end=4.0,
                      force_segments=[[0.0, 5.0], [1.0, -8.0], [2.5, 1.0]])
        coarse = simulate(make_scenario(sample_dt=0.5, **kwargs))
        fine = simulate(make_scenario(sample_dt=0.03, **kwargs))
        ec, ef = energy_summary(coarse), energy_summary(fine)
        for key in ("distance_total", "work_external",
                    "friction_dissipation", "residual"):
            self.assertAlmostEqual(ec[key], ef[key], places=9)
        rows_fine = sample_trajectory(fine)
        rows_coarse = sample_trajectory(coarse)
        self.assertAlmostEqual(rows_fine[-1]["x"], rows_coarse[-1]["x"],
                               places=9)


class TestCli(unittest.TestCase):
    def test_batch_with_failing_scenario(self):
        spec = {
            "scenarios": [
                {"name": "good", "mass": 1.0, "gravity": 10.0,
                 "mu_static": 0.3, "mu_kinetic": 0.2, "t_end": 2.0,
                 "sample_dt": 0.5, "force_segments": [[0.0, 5.0]]},
                {"name": "bad", "mass": 1.0, "gravity": 10.0,
                 "mu_static": 0.1, "mu_kinetic": 0.2, "t_end": 2.0,
                 "sample_dt": 0.5, "force_segments": [[0.0, 5.0]]},
            ]
        }
        with tempfile.TemporaryDirectory() as tmp:
            src = Path(tmp) / "in.json"
            src.write_text(json.dumps(spec), encoding="utf-8")
            out = Path(tmp) / "out"
            proc = subprocess.run(
                [sys.executable, "-m", "friction_motion", str(src),
                 "-o", str(out)],
                capture_output=True, text=True,
            )
            self.assertEqual(proc.returncode, 0, proc.stderr)
            summary = json.loads((out / "summary.json").read_text())
            statuses = {s["name"]: s["status"] for s in summary["scenarios"]}
            self.assertEqual(statuses, {"good": "ok", "bad": "error"})
            self.assertTrue((out / "good_trajectory.csv").exists())
            self.assertTrue((out / "good_events.json").exists())
            self.assertFalse((out / "bad_trajectory.csv").exists())


if __name__ == "__main__":
    unittest.main()
