from __future__ import annotations

from dataclasses import dataclass, field

from .observations import Observation
from .plan import Plan

DECISION_ALTERNATIVE = "alternative"
DECISION_NULL = "null"
DECISION_UNDECIDED = "undecided"

REASON_UPPER = "upper_bound_reached"
REASON_LOWER = "lower_bound_reached"
REASON_MAX = "max_observations_reached"
REASON_EOF = "data_exhausted"


@dataclass(frozen=True)
class Step:
    index: int
    position: int
    value: int
    increment: float
    cumulative: float
    state: str  # "continue", "alternative", "null", "unused"


@dataclass
class ReplayResult:
    sequence_id: str
    steps: list[Step] = field(default_factory=list)
    decision: str = DECISION_UNDECIDED
    reason: str = REASON_EOF
    stop_index: int = 0
    used: int = 0
    unused: int = 0
    final_llr: float = 0.0


def replay_sequence(plan: Plan, sequence_id: str,
                    observations: list[Observation]) -> ReplayResult:
    result = ReplayResult(sequence_id=sequence_id)
    cumulative = 0.0
    decided = False
    for index, obs in enumerate(observations, start=1):
        if decided or index > plan.max_observations:
            result.steps.append(Step(index, obs.position, obs.value,
                                     0.0, cumulative, "unused"))
            continue
        increment = plan.step_success if obs.value == 1 else plan.step_failure
        cumulative += increment
        state = "continue"
        if cumulative >= plan.upper:
            state = DECISION_ALTERNATIVE
            result.decision = DECISION_ALTERNATIVE
            result.reason = REASON_UPPER
            decided = True
        elif cumulative <= plan.lower:
            state = DECISION_NULL
            result.decision = DECISION_NULL
            result.reason = REASON_LOWER
            decided = True
        result.steps.append(Step(index, obs.position, obs.value,
                                 increment, cumulative, state))
        if decided:
            result.stop_index = index
            result.used = index
    total = len(observations)
    if not decided:
        result.used = min(total, plan.max_observations)
        result.stop_index = result.used
        if total >= plan.max_observations:
            result.reason = REASON_MAX
        else:
            result.reason = REASON_EOF
    result.unused = total - result.used
    result.final_llr = cumulative
    return result
