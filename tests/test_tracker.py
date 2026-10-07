import math
import unittest

from gated_particle_tracker.config import Config, ConfigError
from gated_particle_tracker.frames import parse_frames
from gated_particle_tracker.tracker import run_tracking


def make_config(**overrides):
    data = {
        "measurement_variance": 0.5,
        "acceleration_variance": 0.1,
        "initial_velocity_variance": 10.0,
        "gate_threshold": 9.0,
        "missed_penalty": 4.0,
        "new_track_penalty": 4.0,
        "max_missed": 2,
    }
    data.update(overrides)
    return Config.from_dict(data)


def obs(i, x, y):
    return {"id": i, "x": x, "y": y}


class ConfigTests(unittest.TestCase):
    def test_measurement_variance_must_be_positive(self):
        with self.assertRaises(ConfigError):
            make_config(measurement_variance=0.0)
        with self.assertRaises(ConfigError):
            make_config(measurement_variance=-1.0)

    def test_other_variances_may_be_zero(self):
        cfg = make_config(acceleration_variance=0.0, initial_velocity_variance=0.0)
        self.assertEqual(cfg.acceleration_variance, 0.0)

    def test_negative_variance_rejected(self):
        with self.assertRaises(ConfigError):
            make_config(acceleration_variance=-0.1)

    def test_missing_key_rejected(self):
        with self.assertRaises(ConfigError):
            Config.from_dict({})


class FrameValidationTests(unittest.TestCase):
    def test_times_must_strictly_increase(self):
        frames = [
            {"time": 1.0, "observations": []},
            {"time": 1.0, "observations": []},
        ]
        with self.assertRaises(ConfigError):
            parse_frames(frames, 8)

    def test_duplicate_observation_ids_rejected(self):
        frames = [{"time": 0.0, "observations": [obs("a", 0, 0), obs("a", 1, 1)]}]
        with self.assertRaises(ConfigError):
            parse_frames(frames, 8)

    def test_non_finite_coordinates_rejected(self):
        frames = [{"time": 0.0, "observations": [obs("a", float("nan"), 0)]}]
        with self.assertRaises(ConfigError):
            parse_frames(frames, 8)

    def test_observation_limit_enforced(self):
        frames = [
            {"time": 0.0, "observations": [obs(str(i), i, 0) for i in range(9)]}
        ]
        with self.assertRaises(ConfigError):
            parse_frames(frames, 8)

    def test_empty_frames_allowed(self):
        frames = [{"time": 0.0}, {"time": 1.0, "observations": []}]
        parsed = parse_frames(frames, 8)
        self.assertEqual(len(parsed), 2)
        self.assertEqual(parsed[0].observations, ())


class TrackingTests(unittest.TestCase):
    def test_crossing_targets_keep_identity(self):
        # Two targets cross at t=2; constant-velocity prediction should
        # keep the identities straight through the crossing.
        frames = []
        for k in range(5):
            t = float(k)
            frames.append(
                {
                    "time": t,
                    "observations": [
                        obs("left", t, 0.0),
                        obs("right", 4.0 - t, 1.0),
                    ],
                }
            )
        parsed = parse_frames(frames, 8)
        result = run_tracking(parsed, make_config())
        self.assertEqual(len(result["tracks"]), 2)
        for frame_result in result["frames"]:
            matches = [
                d for d in frame_result["decisions"] if d["type"] == "match"
            ]
            if frame_result["time"] == 0.0:
                continue
            self.assertEqual(len(matches), 2)
        # Track 1 starts at x=0 moving +x; track 2 starts at x=4 moving -x.
        final = result["frames"][-1]["states"]
        positions = sorted(s["position"][0] for s in final)
        self.assertAlmostEqual(positions[0], 0.0, delta=0.05)
        self.assertAlmostEqual(positions[1], 4.0, delta=0.05)

    def test_empty_frame_marks_missed_and_keeps_prediction(self):
        frames = parse_frames(
            [
                {"time": 0.0, "observations": [obs("a", 0.0, 0.0)]},
                {"time": 1.0, "observations": []},
                {"time": 2.0, "observations": [obs("b", 2.0, 0.0)]},
            ],
            8,
        )
        result = run_tracking(frames, make_config())
        frame1 = result["frames"][1]
        self.assertEqual(frame1["decisions"][0]["type"], "missed")
        self.assertEqual(frame1["states"][0]["consecutive_misses"], 1)
        # Track survives the empty frame and is matched again at t=2.
        frame2 = result["frames"][2]
        self.assertEqual(frame2["decisions"][0]["type"], "match")
        self.assertEqual(len(result["tracks"]), 1)

    def test_track_ends_after_max_missed(self):
        frames = parse_frames(
            [
                {"time": 0.0, "observations": [obs("a", 0.0, 0.0)]},
                {"time": 1.0, "observations": []},
                {"time": 2.0, "observations": []},
                {"time": 3.0, "observations": []},
            ],
            8,
        )
        result = run_tracking(frames, make_config(max_missed=2))
        track = result["tracks"][0]
        self.assertEqual(track["status"], "ended")
        self.assertEqual(track["end_time"], 2.0)
        self.assertEqual(result["frames"][3]["states"], [])

    def test_observation_order_does_not_change_result(self):
        base = [
            {"time": 0.0, "observations": [obs("a", 0.0, 0.0), obs("b", 5.0, 0.0)]},
            {"time": 1.0, "observations": [obs("c", 1.0, 0.0), obs("d", 6.0, 0.0)]},
        ]
        shuffled = [
            {"time": 0.0, "observations": [obs("b", 5.0, 0.0), obs("a", 0.0, 0.0)]},
            {"time": 1.0, "observations": [obs("d", 6.0, 0.0), obs("c", 1.0, 0.0)]},
        ]
        cfg = make_config()
        r1 = run_tracking(parse_frames(base, 8), cfg)
        r2 = run_tracking(parse_frames(shuffled, 8), cfg)
        # Decisions reference observation ids, so they must be identical.
        self.assertEqual(r1, r2)

    def test_active_track_limit_fails_loudly(self):
        frames = parse_frames(
            [{"time": 0.0, "observations": [obs(str(i), 10.0 * i, 0.0) for i in range(8)]}],
            8,
        )
        with self.assertRaises(ConfigError):
            run_tracking(frames, make_config(max_active_tracks=4))

    def test_gate_blocks_far_observation_and_spawns_track(self):
        frames = parse_frames(
            [
                {"time": 0.0, "observations": [obs("a", 0.0, 0.0)]},
                {"time": 1.0, "observations": [obs("b", 100.0, 0.0)]},
            ],
            8,
        )
        result = run_tracking(frames, make_config())
        frame1 = result["frames"][1]
        types = sorted(d["type"] for d in frame1["decisions"])
        self.assertEqual(types, ["missed", "new_track"])
        self.assertEqual(len(result["tracks"]), 2)

    def test_output_structure(self):
        frames = parse_frames(
            [{"time": 0.0, "observations": [obs("a", 1.0, 2.0)]}], 8
        )
        result = run_tracking(frames, make_config())
        state = result["frames"][0]["states"][0]
        self.assertEqual(state["position"], [1.0, 2.0])
        cov = state["position_covariance"]
        self.assertEqual(len(cov), 2)
        self.assertTrue(all(math.isfinite(v) for row in cov for v in row))
        summary = result["tracks"][0]
        self.assertEqual(summary["status"], "active_at_end")
        self.assertEqual(summary["start_time"], 0.0)


if __name__ == "__main__":
    unittest.main()
