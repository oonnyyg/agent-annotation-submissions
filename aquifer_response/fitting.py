"""Weighted squared-error scoring of user-supplied parameter candidates.

Only the finite candidate list given by the user is evaluated; no
continuous optimization is performed. Ties keep the input order
(Python's sort is stable).
"""

from __future__ import annotations

from dataclasses import dataclass

from .model import (
    Aquifer,
    EvaluationStats,
    ObservationPoint,
    Well,
    WellFunctionConfig,
    well_contribution_at,
)


class FittingError(Exception):
    """Raised for invalid observation or candidate input."""


@dataclass(frozen=True)
class Observation:
    point: str
    time: float
    drawdown: float
    weight: float


@dataclass(frozen=True)
class Candidate:
    name: str
    transmissivity: float
    storativity: float


@dataclass(frozen=True)
class CandidateScore:
    name: str
    transmissivity: float
    storativity: float
    weighted_sse: float
    weighted_rmse: float
    observation_count: int
    rank: int


def validate_observations(
    observations: list[Observation], points: list[ObservationPoint]
) -> None:
    known = {p.name for p in points}
    for obs in observations:
        if obs.point not in known:
            raise FittingError(
                f"observation references unknown point {obs.point!r}"
            )
        if obs.weight <= 0.0:
            raise FittingError(
                f"observation weight must be positive (point {obs.point!r}, "
                f"time {obs.time})"
            )


def score_candidates(
    candidates: list[Candidate],
    observations: list[Observation],
    wells: list[Well],
    points: list[ObservationPoint],
    config: WellFunctionConfig | None = None,
    stats: EvaluationStats | None = None,
) -> list[CandidateScore]:
    """Score every candidate and return them sorted by weighted SSE.

    Stable ordering: candidates with equal error keep input order.
    """
    validate_observations(observations, points)
    point_by_name = {p.name: p for p in points}
    scores: list[CandidateScore] = []
    for candidate in candidates:
        aquifer = Aquifer(candidate.transmissivity, candidate.storativity)
        wsse = 0.0
        for obs in observations:
            point = point_by_name[obs.point]
            predicted = sum(
                well_contribution_at(aquifer, well, point, obs.time, config, stats)
                for well in wells
            )
            residual = predicted - obs.drawdown
            wsse += obs.weight * residual * residual
        count = len(observations)
        wrmse = (wsse / count) ** 0.5 if count else 0.0
        scores.append(
            CandidateScore(
                name=candidate.name,
                transmissivity=candidate.transmissivity,
                storativity=candidate.storativity,
                weighted_sse=wsse,
                weighted_rmse=wrmse,
                observation_count=count,
                rank=0,
            )
        )
    ranked = sorted(scores, key=lambda s: s.weighted_sse)
    return [
        CandidateScore(**{**s.__dict__, "rank": index + 1})
        for index, s in enumerate(ranked)
    ]
