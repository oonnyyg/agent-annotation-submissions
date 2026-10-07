from __future__ import annotations

from dataclasses import dataclass, field

from .plan import Plan


@dataclass
class StepProbability:
    step: int
    stop_alternative: float
    stop_null: float
    survival: float  # probability of still being undecided after this step


@dataclass
class ExactResult:
    true_p: float
    steps: list[StepProbability] = field(default_factory=list)
    undecided_at_max: float = 0.0
    expected_sample: float = 0.0
    total_stop_probability: float = 0.0


def exact_stopping(plan: Plan) -> ExactResult:
    """Exact recursion over unabsorbed paths under plan.true_p.

    The cumulative log-likelihood ratio after n observations depends only
    on the number of successes k, so states are (n, k). Absorbed mass
    never propagates further, keeping total probability conserved.
    """
    p = plan.true_p
    q = 1.0 - p
    s1 = plan.step_success
    s0 = plan.step_failure
    result = ExactResult(true_p=p)
    # Unabsorbed distribution over success counts at the current step.
    alive: dict[int, float] = {0: 1.0}
    for n in range(1, plan.max_observations + 1):
        result.expected_sample += sum(alive.values())
        next_alive: dict[int, float] = {}
        stop_alt = 0.0
        stop_null = 0.0
        for k, mass in alive.items():
            for dk, prob in ((1, p), (0, q)):
                moved = mass * prob
                if moved == 0.0:
                    continue
                llr = (k + dk) * s1 + (n - k - dk) * s0
                if llr >= plan.upper:
                    stop_alt += moved
                elif llr <= plan.lower:
                    stop_null += moved
                else:
                    key = k + dk
                    next_alive[key] = next_alive.get(key, 0.0) + moved
        survival = sum(next_alive.values())
        result.steps.append(StepProbability(n, stop_alt, stop_null, survival))
        result.total_stop_probability += stop_alt + stop_null
        alive = next_alive
    result.undecided_at_max = sum(alive.values())
    return result
