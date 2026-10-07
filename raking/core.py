"""Iterative proportional fitting (raking) core."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .inputs import Margin, Sample, category_key

STATUS_CONVERGED = "converged"
STATUS_NOT_CONVERGED = "not_converged"
STATUS_UNREACHABLE = "unreachable"
STATUS_OVERFLOW = "overflow"


@dataclass
class Failure:
    iteration: int  # 0 means detected before the first pass
    margin: str
    category: str
    reason: str


@dataclass
class RoundRecord:
    iteration: int
    max_residual: float
    margin: str
    category: str


@dataclass
class RakingResult:
    status: str
    weights: list[float]
    iterations: int
    history: list[RoundRecord] = field(default_factory=list)
    failures: list[Failure] = field(default_factory=list)
    achieved: dict[str, dict[str, float]] = field(default_factory=dict)

    @property
    def converged(self) -> bool:
        return self.status == STATUS_CONVERGED


def _margin_totals(weights: list[float], samples: list[Sample],
                   margin: Margin) -> dict[str, float]:
    totals = {key: 0.0 for key in margin.targets}
    for weight, sample in zip(weights, samples):
        key = category_key(sample.fields, margin.fields)
        if key in totals:
            totals[key] += weight
    return totals


def _max_residual(weights: list[float], samples: list[Sample],
                  margins: list[Margin]) -> tuple[float, str, str, dict]:
    """Largest |achieved - target| / max(target, 1) across ALL margins."""
    worst = (-1.0, "", "")
    achieved: dict[str, dict[str, float]] = {}
    for margin in margins:
        totals = _margin_totals(weights, samples, margin)
        achieved[margin.name] = totals
        for key, target in margin.targets.items():
            scale = max(target, 1.0)
            residual = abs(totals.get(key, 0.0) - target) / scale
            if residual > worst[0]:
                worst = (residual, margin.name, key)
    return worst[0], worst[1], worst[2], achieved


def rake(samples: list[Sample], margins: list[Margin], tolerance: float,
         max_iterations: int) -> RakingResult:
    weights = [s.initial_weight for s in samples]
    history: list[RoundRecord] = []
    failures: list[Failure] = []

    # Structural check: a positive target with no supporting sample can never
    # be reached (we do not fabricate sample units).
    for margin in margins:
        support = {category_key(s.fields, margin.fields) for s in samples}
        for key, target in margin.targets.items():
            if target > 0.0 and key not in support:
                failures.append(Failure(
                    0, margin.name, key,
                    "positive target has no supporting sample units"))
    if failures:
        return RakingResult(STATUS_UNREACHABLE, weights, 0, history, failures)

    for iteration in range(1, max_iterations + 1):
        for margin in margins:
            totals = _margin_totals(weights, samples, margin)
            for key, target in margin.targets.items():
                current = totals[key]
                if target == 0.0:
                    if current != 0.0:
                        for i, sample in enumerate(samples):
                            if category_key(sample.fields,
                                            margin.fields) == key:
                                weights[i] = 0.0
                    continue
                if current == 0.0:
                    failures.append(Failure(
                        iteration, margin.name, key,
                        "positive target but current class weight is zero "
                        "(zeroed out by a conflicting zero target)"))
                    return RakingResult(STATUS_UNREACHABLE, weights,
                                        iteration, history, failures)
                ratio = target / current
                for i, sample in enumerate(samples):
                    if category_key(sample.fields, margin.fields) == key:
                        weights[i] *= ratio
                        if not math.isfinite(weights[i]):
                            failures.append(Failure(
                                iteration, margin.name, key,
                                "weight became non-finite (overflow)"))
                            return RakingResult(STATUS_OVERFLOW, weights,
                                                iteration, history, failures)

        # Convergence is judged on every margin, not only the last adjusted.
        residual, mname, cname, achieved = _max_residual(weights, samples,
                                                         margins)
        history.append(RoundRecord(iteration, residual, mname, cname))
        if residual <= tolerance:
            return RakingResult(STATUS_CONVERGED, weights, iteration,
                                history, failures, achieved)

    _, _, _, achieved = _max_residual(weights, samples, margins)
    return RakingResult(STATUS_NOT_CONVERGED, weights, max_iterations,
                        history, failures, achieved)


def effective_sample_size(weights: list[float]) -> float:
    total = math.fsum(weights)
    squares = math.fsum(w * w for w in weights)
    if squares == 0.0:
        return 0.0
    return total * total / squares
