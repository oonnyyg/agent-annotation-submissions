import math
import unittest

from seqaudit.exact import exact_stopping
from seqaudit.plan import InputError, Observation, Plan, _require
from seqaudit.replay import replay


def make_plan(**overrides) -> Plan:
    base = dict(plan_id="t", p0=0.25, p1=0.75, lower=-2.0, upper=2.0,
                max_observations=10, true_p=0.5)
    base.update(overrides)
    return Plan(**base)


def make_plan_check(**kw):
    plan = make_plan(**kw)
    _require(0.0 < plan.p0 < 1.0, "p0")
    _require(0.0 < plan.p1 < 1.0, "p1")
    _require(plan.p1 > plan.p0, "order")
    _require(plan.lower < 0, "lower")
    _require(plan.upper > 0, "upper")
    _require(1 <= plan.max_observations <= 200, "cap")
    return plan


def obs(outcomes, sid="s"):
    return [Observation(sid, i + 1, o) for i, o in enumerate(outcomes)]


class PlanValidationTests(unittest.TestCase):
    def test_boundary_rates_rejected(self):
        for bad in (0.0, 1.0, -0.1, 1.1):
            with self.assertRaises(InputError):
                make_plan_check(p0=bad)

    def test_alternative_must_exceed_null(self):
        with self.assertRaises(InputError):
            make_plan_check(p0=0.6, p1=0.4)

    def test_bounds_sign_and_cap(self):
        with self.assertRaises(InputError):
            make_plan_check(lower=0.5)
        with self.assertRaises(InputError):
            make_plan_check(upper=-0.5)
        with self.assertRaises(InputError):
            make_plan_check(max_observations=201)


class ReplayTests(unittest.TestCase):
    def test_boundary_equality_triggers_decision(self):
        plan = make_plan()
        step = plan.step_success
        plan = make_plan(upper=2 * step, lower=-2.0)
        result = replay(plan, obs([1, 1, 0, 0, 0]))
        self.assertEqual(result.decision, "accept_alternative")
        self.assertEqual(result.stop_reason, "upper_bound_reached")
        self.assertEqual(result.used, 2)
        self.assertEqual(result.unused, 3)
        self.assertAlmostEqual(result.final_llr, 2 * step)

    def test_first_crossing_only_and_unused(self):
        plan = make_plan()
        result = replay(plan, obs([0, 0, 0, 1, 1, 1, 1]))
        self.assertEqual(result.decision, "accept_null")
        self.assertEqual(result.stop_reason, "lower_bound_reached")
        self.assertEqual(result.used, 2)
        self.assertEqual(result.unused, 5)
        self.assertEqual(len(result.steps), 2)

    def test_cap_boundary_checked_before_undecided(self):
        plan = make_plan(max_observations=2)
        result = replay(plan, obs([1, 1, 1, 1]))
        self.assertEqual(result.decision, "accept_alternative")
        self.assertEqual(result.used, 2)
        self.assertEqual(result.unused, 2)

    def test_cap_undecided(self):
        plan = make_plan(max_observations=3)
        result = replay(plan, obs([1, 0, 1, 0, 1]))
        self.assertEqual(result.decision, "undecided")
        self.assertEqual(result.stop_reason, "max_observations_reached")
        self.assertEqual(result.used, 3)
        self.assertEqual(result.unused, 2)

    def test_data_exhausted_no_forced_decision(self):
        plan = make_plan(max_observations=20)
        result = replay(plan, obs([1, 0, 1]))
        self.assertEqual(result.decision, "undecided")
        self.assertEqual(result.stop_reason, "data_exhausted")
        self.assertEqual(result.used, 3)
        self.assertEqual(result.unused, 0)
        self.assertAlmostEqual(result.final_llr,
                               2 * plan.step_success + plan.step_failure)


class ExactTests(unittest.TestCase):
    def test_probability_conserved_every_step(self):
        plan = make_plan(max_observations=50, true_p=0.4)
        result = exact_stopping(plan)
        cum_null = cum_alt = 0.0
        for s in result.steps:
            cum_null += s.prob_stop_null
            cum_alt += s.prob_stop_alternative
            self.assertAlmostEqual(cum_null + cum_alt + s.prob_running, 1.0, places=9)
        self.assertAlmostEqual(result.total_probability, 1.0, places=9)

    def test_no_stop_when_bounds_unreachable(self):
        plan = make_plan(lower=-100.0, upper=100.0, max_observations=5)
        result = exact_stopping(plan)
        self.assertEqual(result.prob_stop_null, 0.0)
        self.assertEqual(result.prob_stop_alternative, 0.0)
        self.assertAlmostEqual(result.prob_undecided, 1.0)
        self.assertAlmostEqual(result.expected_sample_size, 5.0)

    def test_expected_sample_size_matches_bruteforce(self):
        plan = make_plan(max_observations=8, true_p=0.6)
        exact = exact_stopping(plan)
        total = 0.0
        prob = 0.0
        for mask in range(2 ** 8):
            llr, p_path, stopped_at = 0.0, 1.0, None
            for i in range(8):
                bit = (mask >> i) & 1
                p_path *= plan.true_p if bit else 1 - plan.true_p
                if stopped_at is None:
                    llr += plan.step_success if bit else plan.step_failure
                    if llr >= plan.upper or llr <= plan.lower:
                        stopped_at = i + 1
            total += p_path * (stopped_at if stopped_at else 8)
        self.assertAlmostEqual(exact.expected_sample_size, total, places=9)

    def test_high_true_p_favors_alternative(self):
        plan = make_plan(true_p=0.9, max_observations=100)
        result = exact_stopping(plan)
        self.assertGreater(result.prob_stop_alternative, result.prob_stop_null)


if __name__ == "__main__":
    unittest.main()
