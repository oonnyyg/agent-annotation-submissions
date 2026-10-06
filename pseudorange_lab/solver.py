"""Weighted iterative least-squares position and clock solver.

Model: predicted pseudorange = geometric range + common clock offset (m).
Unknowns: receiver (x, y, z) and clock offset, estimated jointly.
Weights: 1 / sigma^2 from the observation CSV (never rescaled).
"""
from __future__ import annotations

import math
from dataclasses import dataclass

from .data import Observation
from .errors import ConvergenceError, GeometryError

MAX_ITERATIONS = 50
CONVERGENCE_TOL_M = 1e-6
RANK_TOL = 1e-12


@dataclass
class Solution:
    position_m: tuple[float, float, float]
    clock_m: float
    iterations: int
    residuals_m: list[float]
    weighted_residual_sum_squares: float
    covariance: list[list[float]]
    sat_ids: list[str]


def _solve_normal_equations(
    matrix: list[list[float]], rhs: list[float]
) -> tuple[list[float], list[list[float]]]:
    """Solve matrix*x = rhs and return (x, inverse) via Gauss-Jordan.

    Raises GeometryError if a pivot indicates rank deficiency.
    """
    n = len(matrix)
    aug = [row[:] + [1.0 if i == j else 0.0 for j in range(n)] + [rhs[i]]
           for i, row in enumerate(matrix)]
    scale = max(abs(matrix[i][i]) for i in range(n)) or 1.0
    for col in range(n):
        pivot = max(range(col, n), key=lambda r: abs(aug[r][col]))
        if abs(aug[pivot][col]) <= RANK_TOL * scale:
            raise GeometryError(
                "design matrix is rank deficient (degenerate geometry)"
            )
        aug[col], aug[pivot] = aug[pivot], aug[col]
        inv_pivot = 1.0 / aug[col][col]
        aug[col] = [v * inv_pivot for v in aug[col]]
        for row in range(n):
            if row != col and aug[row][col] != 0.0:
                factor = aug[row][col]
                aug[row] = [v - factor * p for v, p in zip(aug[row], aug[col])]
    solution = [aug[i][2 * n] for i in range(n)]
    inverse = [row[n:2 * n] for row in aug]
    return solution, inverse


def solve_epoch(
    observations: list[Observation],
    initial_position_m: tuple[float, float, float],
    initial_clock_m: float,
) -> Solution:
    """Estimate receiver position and clock for one epoch."""
    if len(observations) < 4:
        raise GeometryError("need at least 4 satellites")
    state = [initial_position_m[0], initial_position_m[1],
             initial_position_m[2], initial_clock_m]
    covariance: list[list[float]] | None = None
    residuals: list[float] = []
    for iterations in range(1, MAX_ITERATIONS + 1):
        normal = [[0.0] * 4 for _ in range(4)]
        rhs = [0.0] * 4
        residuals = []
        for obs in observations:
            dx = obs.position_m[0] - state[0]
            dy = obs.position_m[1] - state[1]
            dz = obs.position_m[2] - state[2]
            rng = math.sqrt(dx * dx + dy * dy + dz * dz)
            if rng == 0.0:
                raise GeometryError(
                    "initial/iterated position coincides with satellite "
                    f"{obs.sat_id!r}"
                )
            row = [-dx / rng, -dy / rng, -dz / rng, 1.0]
            weight = 1.0 / (obs.sigma_m * obs.sigma_m)
            resid = obs.pseudorange_m - (rng + state[3])
            residuals.append(resid)
            for i in range(4):
                rhs[i] += weight * row[i] * resid
                for j in range(i, 4):
                    normal[i][j] += weight * row[i] * row[j]
        for i in range(4):
            for j in range(i):
                normal[i][j] = normal[j][i]
        step, covariance = _solve_normal_equations(normal, rhs)
        for i in range(4):
            state[i] += step[i]
        if not all(math.isfinite(v) for v in state):
            raise ConvergenceError("iteration produced non-finite state")
        if max(abs(s) for s in step) < CONVERGENCE_TOL_M:
            residuals = [
                obs.pseudorange_m
                - (math.dist(obs.position_m, state[:3]) + state[3])
                for obs in observations
            ]
            wrss = sum(
                (r / obs.sigma_m) ** 2
                for r, obs in zip(residuals, observations)
            )
            return Solution(
                position_m=(state[0], state[1], state[2]),
                clock_m=state[3],
                iterations=iterations,
                residuals_m=residuals,
                weighted_residual_sum_squares=wrss,
                covariance=covariance,
                sat_ids=[o.sat_id for o in observations],
            )
    raise ConvergenceError(
        f"least squares did not converge in {MAX_ITERATIONS} iterations"
    )
