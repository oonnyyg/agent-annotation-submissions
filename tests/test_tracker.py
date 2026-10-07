import math
import os
import random
import sys
import unittest

sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))

from tracker import TrackerConfig, TrackerError, track_frames
from tracker.assignment import hungarian_square, min_cost_assignment
from tracker.kalman import KalmanState


def make_config(**overrides):
    params = dict(
        meas_var=1.0,
        accel_var=0.1,
        init_vel_var=10.0,
        gate_threshold=9.0,
        miss_penalty=5.0,
        new_penalty=5.0,
        max_misses=2,
    )
    params.update(overrides)
    return TrackerConfig(**params)


def obs(obs_id, x, y):
    return {"id": obs_id, "x": x, "y": y}


class ConfigValidationTest(unittest.TestCase):
    def test_meas_var_must_be_positive(self):
        with self.assertRaises(TrackerError):
            make_config(meas_var=0.0)
        with self.assertRaises(TrackerError):
            make_config(meas_var=-1.0)

    def test_other_variances_non_negative(self):
        make_config(accel_var=0.0, init_vel_var=0.0)
        with self.assertRaises(TrackerError):
            make_config(accel_var=-0.1)
        with self.assertRaises(TrackerError):
            make_config(init_vel_var=-0.1)

    def test_non_finite_rejected(self):
        with self.assertRaises(TrackerError):
            make_config(meas_var=math.inf)
        with self.assertRaises(TrackerError):
            make_config(accel_var=math.nan)


class FrameValidationTest(unittest.TestCase):
    def test_times_must_increase(self):
        frames = [
            {"time": 1.0, "observations": []},
            {"time": 1.0, "observations": []},
        ]
        with self.assertRaises(TrackerError):
            track_frames(frames, make_config())

    def test_duplicate_observation_ids_rejected(self):
        frames = [{"time": 0.0, "observations": [obs(1, 0, 0), obs(1, 1, 1)]}]
        with self.assertRaises(TrackerError):
            track_frames(frames, make_config())

    def test_non_finite_coordinates_rejected(self):
        frames = [{"time": 0.0, "observations": [obs(1, math.nan, 0.0)]}]
        with self.assertRaises(TrackerError):
            track_frames(frames, make_config())

    def test_empty_frames_allowed(self):
        frames = [{"time": float(i), "observations": []} for i in range(4)]
        result = track_frames(frames, make_config())
        self.assertEqual(len(result["frames"]), 4)
        self.assertEqual(result["summary"], [])

    def test_observation_limit_enforced(self):
        frames = [
            {"time": 0.0, "observations": [obs(i, float(i), 0.0) for i in range(9)]}
        ]
        with self.assertRaises(TrackerError):
            track_frames(frames, make_config())

    def test_track_limit_enforced(self):
        frames = [
            {"time": 0.0, "observations": [obs(i, float(i) * 100.0, 0.0) for i in range(8)]},
            {"time": 1.0, "observations": [obs(100, 450.0, 0.0)]},
        ]
        with self.assertRaises(TrackerError):
            track_frames(frames, make_config())


class KalmanTest(unittest.TestCase):
    def test_new_track_state(self):
        state = KalmanState.from_measurement(3.0, -2.0, 1.5, 4.0)
        self.assertEqual(state.position, (3.0, -2.0))
        self.assertEqual(state.position_covariance(), [[1.5, 0.0], [0.0, 1.5]])
        self.assertEqual(state.x_axis.vel, 0.0)
        self.assertEqual(state.x_axis.p11, 4.0)

    def test_predict_constant_velocity(self):
        state = KalmanState.from_measurement(0.0, 0.0, 1.0, 1.0)
        state.predict(1.0, 0.0)
        state.update(1.0, 0.0, 1.0)
        self.assertAlmostEqual(state.x_axis.vel, 1.0 / 3.0)
        state.predict(2.0, 0.0)
        self.assertAlmostEqual(state.position[0], 4.0 / 3.0)
        self.assertGreater(state.x_axis.p00, 0.0)

    def test_zero_dt_prediction_keeps_position(self):
        state = KalmanState.from_measurement(5.0, 6.0, 1.0, 1.0)
        state.predict(0.0, 0.5)
        self.assertEqual(state.position, (5.0, 6.0))

    def test_mahalanobis_at_prediction_is_zero(self):
        state = KalmanState.from_measurement(1.0, 2.0, 1.0, 1.0)
        self.assertAlmostEqual(state.mahalanobis_sq(1.0, 2.0, 1.0), 0.0)


class AssignmentTest(unittest.TestCase):
    def test_global_not_greedy(self):
        # Greedy would take (0,0)=1 then (1,1)=100; optimum is 2+2.
        cost = [[1.0, 2.0], [2.0, 100.0]]
        self.assertEqual(min_cost_assignment(cost), [1, 0])

    def test_forbidden_edges(self):
        inf = math.inf
        cost = [[inf, 1.0], [1.0, inf]]
        self.assertEqual(min_cost_assignment(cost), [1, 0])

    def test_tie_break_prefers_lowest_indices(self):
        cost = [[1.0, 1.0], [1.0, 1.0]]
        self.assertEqual(min_cost_assignment(cost), [0, 1])

    def test_square_solver(self):
        self.assertEqual(hungarian_square([[0.0, 1.0], [1.0, 0.0]]), [0, 1])


class TrackingTest(unittest.TestCase):
    def test_crossing_targets_keep_identity(self):
        # Two targets cross at t=2; constant-velocity prediction plus
        # global assignment must keep the identities straight.
        frames = []
        for k in range(5):
            t = float(k)
            frames.append(
                {
                    "time": t,
                    "observations": [
                        obs(10 * k, 2.0 * k, 0.0),
                        obs(10 * k + 1, 8.0 - 2.0 * k, 0.0),
                    ],
                }
            )
        result = track_frames(frames, make_config())
        self.assertEqual(len(result["summary"]), 2)
        for frame_out in result["frames"]:
            pairs = {a["track_id"]: a["observation_id"] for a in frame_out["associations"]}
            self.assertEqual(len(pairs), 2)
        last = result["frames"][-1]
        xs = {t["track_id"]: t["x"] for t in last["tracks"]}
        self.assertGreater(xs[1], 6.0)  # track 1 moves +x
        self.assertLess(xs[2], 2.0)  # track 2 moves -x

    def test_missed_detection_then_end(self):
        frames = [
            {"time": 0.0, "observations": [obs(1, 0.0, 0.0)]},
            {"time": 1.0, "observations": []},
            {"time": 2.0, "observations": []},
            {"time": 3.0, "observations": []},
        ]
        result = track_frames(frames, make_config(max_misses=2))
        self.assertEqual(result["frames"][1]["tracks"][0]["misses"], 1)
        self.assertEqual(result["frames"][2]["ended_track_ids"], [1])
        self.assertEqual(result["frames"][3]["tracks"], [])
        self.assertEqual(result["summary"][0]["end_frame"], 0)

    def test_unmatched_observation_creates_track(self):
        frames = [
            {"time": 0.0, "observations": [obs(1, 0.0, 0.0)]},
            {"time": 1.0, "observations": [obs(2, 0.1, 0.0), obs(3, 50.0, 0.0)]},
        ]
        result = track_frames(frames, make_config())
        self.assertEqual(result["frames"][1]["new_track_ids"], [2])

    def test_observation_permutation_invariance(self):
        rng = random.Random(7)
        base = [
            {"time": 0.0, "observations": [obs(1, 0.0, 0.0), obs(2, 5.0, 1.0), obs(3, 9.0, 0.0)]},
            {"time": 1.0, "observations": [obs(4, 1.0, 0.1), obs(5, 4.0, 1.1), obs(6, 10.0, 0.2)]},
            {"time": 2.0, "observations": [obs(7, 2.0, 0.0), obs(8, 3.0, 1.2)]},
        ]
        reference = track_frames(base, make_config())
        for _ in range(10):
            shuffled = [
                {"time": f["time"], "observations": rng.sample(f["observations"], len(f["observations"]))}
                for f in base
            ]
            self.assertEqual(track_frames(shuffled, make_config()), reference)

    def test_gating_blocks_far_association(self):
        frames = [
            {"time": 0.0, "observations": [obs(1, 0.0, 0.0)]},
            {"time": 1.0, "observations": [obs(2, 100.0, 0.0)]},
        ]
        result = track_frames(frames, make_config())
        self.assertEqual(result["frames"][1]["new_track_ids"], [2])
        self.assertEqual(result["frames"][1]["tracks"][0]["misses"], 1)

    def test_output_structure(self):
        frames = [{"time": 0.0, "observations": [obs(1, 1.0, 2.0)]}]
        result = track_frames(frames, make_config())
        track_out = result["frames"][0]["tracks"][0]
        self.assertEqual(set(track_out), {"track_id", "x", "y", "covariance", "misses"})
        self.assertEqual(len(track_out["covariance"]), 2)
        self.assertEqual(
            set(result["summary"][0]), {"track_id", "start_frame", "end_frame", "length"}
        )


if __name__ == "__main__":
    unittest.main()
