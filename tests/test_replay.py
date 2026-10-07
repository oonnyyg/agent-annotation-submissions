import unittest

from seqaudit.observations import Observation
from seqaudit.plan import build_plan
from seqaudit.replay import (DECISION_ALTERNATIVE, DECISION_NULL,
                             DECISION_UNDECIDED, REASON_EOF, REASON_MAX,
                             replay_sequence)


def make_plan(**overrides):
    data = {"p0": 0.4, "p1": 0.6, "lower": -1.0, "upper": 1.0,
            "max_observations": 10, "true_p": 0.5}
    data.update(overrides)
    return build_plan(data)


def obs(values):
    return [Observation("s", i + 1, v) for i, v in enumerate(values)]


class ReplayTests(unittest.TestCase):
    def test_upper_crossing_stops_and_marks_unused(self):
        plan = make_plan()
        result = replay_sequence(plan, "s", obs([1, 1, 1, 1, 0, 0]))
        self.assertEqual(result.decision, DECISION_ALTERNATIVE)
        self.assertEqual(result.stop_index, 3)
        self.assertEqual(result.used, 3)
        self.assertEqual(result.unused, 3)
        self.assertEqual(result.steps[-1].state, "unused")

    def test_lower_crossing(self):
        plan = make_plan()
        result = replay_sequence(plan, "s", obs([0, 0, 0, 1]))
        self.assertEqual(result.decision, DECISION_NULL)
        self.assertEqual(result.stop_index, 3)
        self.assertEqual(result.unused, 1)

    def test_boundary_checked_at_max_before_undecided(self):
        plan = make_plan(max_observations=3)
        result = replay_sequence(plan, "s", obs([1, 1, 1]))
        self.assertEqual(result.decision, DECISION_ALTERNATIVE)
        self.assertEqual(result.stop_index, 3)

    def test_undecided_at_max(self):
        plan = make_plan(max_observations=2)
        result = replay_sequence(plan, "s", obs([1, 0, 1, 0]))
        self.assertEqual(result.decision, DECISION_UNDECIDED)
        self.assertEqual(result.reason, REASON_MAX)
        self.assertEqual(result.used, 2)
        self.assertEqual(result.unused, 2)

    def test_early_eof_stays_undecided(self):
        plan = make_plan()
        result = replay_sequence(plan, "s", obs([1, 0]))
        self.assertEqual(result.decision, DECISION_UNDECIDED)
        self.assertEqual(result.reason, REASON_EOF)
        self.assertEqual(result.used, 2)
        self.assertAlmostEqual(result.final_llr,
                               plan.step_success + plan.step_failure)

    def test_first_crossing_only(self):
        plan = make_plan()
        result = replay_sequence(plan, "s", obs([1, 1, 1, 0, 0, 0, 0, 0]))
        self.assertEqual(result.decision, DECISION_ALTERNATIVE)
        self.assertEqual(result.stop_index, 3)
        self.assertEqual(result.unused, 5)


if __name__ == "__main__":
    unittest.main()
