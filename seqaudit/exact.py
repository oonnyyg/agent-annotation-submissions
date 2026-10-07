"""Exact stopping-probability recursion over unstopped cumulative-LLR states.

The distribution of the cumulative log-likelihood ratio among paths that have
not yet stopped is propagated step by step under the user-specified true
success rate. Stopped paths are removed and never propagate further, so total
probability (still-running + stopped at either boundary) is conserved at 1.
"""

from __future__ import annotations

from dataclasses import dataclass, field

from .plan import Plan


@dataclass(frozen=True)
class StepProbability:
    step: int
    prob_stop_null: float
    prob_stop_alternative: float
    prob_running: float


@dataclass
class ExactResult:
    true_p: float
    max_observations: int
    steps: list[StepProbability] = field(default_factory=list)
    prob_stop_null: float = 0.0
    prob_stop_alternative: float = 0.0
    prob_undecided: float = 0.0
    expected_sample_size: float = 0.0

    @property
    def total_probability(self) -> float:
        return self.prob_stop_null + self.prob_stop_alternative + self.prob_undecided


def exact_stopping(plan: Plan) -> ExactResult:
    p = plan.true_p
    # Distribution of cumulative LLR among still-running paths: {llr: prob}.
    running: dict[float, float] = {0.0: 1.0}
    result = ExactResult(true_p=p, max_observations=plan.max_observations)

    for step in range(1, plan.max_observations + 1):
        nxt: dict[float, float] = {}
        stop_null = 0.0
        stop_alt = 0.0
        for llr, prob in running.items():
            for increment, weight in ((plan.step_success, p), (plan.step_failure, 1.0 - p)):
                moved = prob * weight
                if moved == 0.0:
                    continue
                new_llr = llr + increment
                if new_llr >= plan.upper:
                    stop_alt += moved
                elif new_llr <= plan.lower:
                    stop_null += moved
                else:
                    nxt[new_llr] = nxt.get(new_llr, 0.0) + moved
        running = nxt
        still = sum(running.values())
        result.steps.append(StepProbability(step, stop_null, stop_alt, still))
        result.prob_stop_null += stop_null
        result.prob_stop_alternative += stop_alt
        # Truncated expected sample size: sum of P(N >= step) over steps.
        result.expected_sample_size += still + stop_null + stop_alt

    result.prob_undecided = sum(running.values())
    return result
