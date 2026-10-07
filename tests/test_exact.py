import unittest

from seqaudit.exact import exact_stopping
from seqaudit.plan import build_plan


def make_plan(**overrides):
    data = {"p0": 0.4, "p1": 0.6, "lower": -1.5, "upper": 1.5,
            "max_observations": 20, "true_p": 0.5}
    data.update(overrides)
    return build_plan(data)


class ExactTests(unittest.TestCase):
    def test_probability_conservation(self):
        exact = exact_stopping(make_plan())
        total = exact.total_stop_probability + exact.undecided_at_max
        self.assertAlmostEqual(total, 1.0, places=12)
        per_step = sum(s.stop_alternative + s.stop_null for s in exact.steps)
        self.assertAlmostEqual(per_step + exact.undecided_at_max, 1.0,
                               places=12)

    def test_survival_decreases_and_stays_nonnegative(self):
        exact = exact_stopping(make_plan())
        previous = 1.0
        for step in exact.steps:
            self.assertLessEqual(step.survival, previous + 1e-15)
            self.assertGreaterEqual(step.survival, 0.0)
            previous = step.survival
        self.assertAlmostEqual(exact.steps[-1].survival,
                               exact.undecided_at_max, places=15)

    def test_expected_sample_within_bounds(self):
        exact = exact_stopping(make_plan())
        self.assertGreater(exact.expected_sample, 0.0)
        self.assertLessEqual(exact.expected_sample, 20.0)

    def test_equal_bounds_stop_immediately(self):
        # A band narrower than one increment ends on the first observation.
        plan = make_plan(lower=-0.01, upper=0.01)
        exact = exact_stopping(plan)
        first = exact.steps[0]
        self.assertAlmostEqual(first.stop_alternative + first.stop_null,
                               1.0, places=15)
        self.assertAlmostEqual(exact.undecided_at_max, 0.0, places=15)
        self.assertAlmostEqual(exact.expected_sample, 1.0, places=15)

    def test_early_absorption_leaves_no_mass(self):
        plan = make_plan(lower=-0.05, upper=0.05, max_observations=10)
        exact = exact_stopping(plan)
        self.assertAlmostEqual(exact.undecided_at_max, 0.0, places=15)
        for step in exact.steps[1:]:
            self.assertEqual(step.stop_alternative, 0.0)
            self.assertEqual(step.stop_null, 0.0)

    def test_wide_bounds_never_stop(self):
        plan = make_plan(lower=-100.0, upper=100.0)
        exact = exact_stopping(plan)
        self.assertAlmostEqual(exact.undecided_at_max, 1.0, places=15)
        self.assertAlmostEqual(exact.expected_sample, 20.0, places=12)


if __name__ == "__main__":
    unittest.main()
