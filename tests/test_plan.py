import unittest

from seqaudit.errors import PlanError
from seqaudit.plan import build_plan


def valid(**overrides):
    data = {"p0": 0.4, "p1": 0.6, "lower": -2.0, "upper": 2.0,
            "max_observations": 50, "true_p": 0.5}
    data.update(overrides)
    return data


class PlanValidationTests(unittest.TestCase):
    def test_valid_plan(self):
        plan = build_plan(valid())
        self.assertGreater(plan.step_success, 0.0)
        self.assertLess(plan.step_failure, 0.0)

    def test_rates_must_be_strictly_inside_unit_interval(self):
        for key in ("p0", "p1", "true_p"):
            for bad in (0.0, 1.0, -0.1, 1.1):
                with self.assertRaises(PlanError):
                    build_plan(valid(**{key: bad}))

    def test_alternative_must_exceed_null(self):
        with self.assertRaises(PlanError):
            build_plan(valid(p1=0.3))

    def test_bounds_signs(self):
        with self.assertRaises(PlanError):
            build_plan(valid(lower=0.0))
        with self.assertRaises(PlanError):
            build_plan(valid(upper=-1.0))

    def test_max_observations_limit(self):
        with self.assertRaises(PlanError):
            build_plan(valid(max_observations=201))
        with self.assertRaises(PlanError):
            build_plan(valid(max_observations=0))


if __name__ == "__main__":
    unittest.main()
