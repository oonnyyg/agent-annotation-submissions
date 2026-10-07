"""Replay a sequence through the sequential probability ratio test (SPRT)."""

from __future__ import annotations

from dataclasses import dataclass, field

from .plan import Observation, Plan

DECISION_NULL = "accept_null"
DECISION_ALTERNATIVE = "accept_alternative"
DECISION_UNDECIDED = "undecided"

REASON_UPPER = "upper_bound_reached"
REASON_LOWER = "lower_bound_reached"
REASON_CAP = "max_observations_reached"
REASON_EXHAUSTED = "data_exhausted"


@dataclass(frozen=True)
class Step:
    index: int
    position: int
    outcome: int
    llr_increment: float
    cumulative_llr: float
    status: str  # "continue", "accept_null", "accept_alternative", "undecided"


@dataclass
class ReplayResult:
    series_id: str
    decision: str
    stop_reason: str
    used: int
    unused: int
    stop_position: int | None
    final_llr: float
    steps: list[Step] = field(default_factory=list)


def _classify(plan: Plan, llr: float) -> str | None:
    if llr >= plan.upper:
        return DECISION_ALTERNATIVE
    if llr <= plan.lower:
        return DECISION_NULL
    return None


def replay(plan: Plan, observations: list[Observation]) -> ReplayResult:
    """Walk observations in order; only the first boundary crossing decides."""
    llr = 0.0
    steps: list[Step] = []
    decision: str | None = None
    stop_reason: str | None = None
    used = 0

    for obs in observations:
        if used >= plan.max_observations:
            break
        used += 1
        llr += plan.step_success if obs.outcome == 1 else plan.step_failure
        hit = _classify(plan, llr)
        if hit is not None:
            decision = hit
            stop_reason = REASON_UPPER if hit == DECISION_ALTERNATIVE else REASON_LOWER
            steps.append(Step(used, obs.position, obs.outcome,
                              plan.step_success if obs.outcome == 1 else plan.step_failure,
                              llr, hit))
            break
        steps.append(Step(used, obs.position, obs.outcome,
                          plan.step_success if obs.outcome == 1 else plan.step_failure,
                          llr, "continue"))

    if decision is None:
        if used >= plan.max_observations:
            decision = DECISION_UNDECIDED
            stop_reason = REASON_CAP
        else:
            decision = DECISION_UNDECIDED
            stop_reason = REASON_EXHAUSTED

    return ReplayResult(
        series_id=observations[0].series_id if observations else "",
        decision=decision,
        stop_reason=stop_reason,
        used=used,
        unused=len(observations) - used,
        stop_position=steps[-1].position if steps else None,
        final_llr=llr,
        steps=steps,
    )
