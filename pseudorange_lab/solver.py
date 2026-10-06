"""Weighted iterative least-squares solver for one epoch of pseudoranges."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .linalg import invert, is_finite_vector, matrix_rank, solve_linear

MAX_ITERATIONS = 50
CONVERGENCE_TOL_M = 1e-8
MIN_RANGE_M = 1e-3


class SolverError(Exception):
    """Base class for epoch solver failures."""


class RankDeficientError(SolverError):
    """Design matrix is rank deficient (degenerate geometry)."""


class ConvergenceError(SolverError):
    """Iteration failed to converge within the iteration limit."""


class InitialGeometryError(SolverError):
    """Invalid inputs, e.g. initial position coincides with a satellite."""


@dataclass(frozen=True)
class SatelliteObs:
    sat_id: str
    x: float
    y: float
    z: float
    pseudorange: float
    sigma: float

    def validate(self) -> None:
        values = [self.x, self.y, self.z, self.pseudorange, self.sigma]
        if not is_finite_vector(values):
            raise InitialGeometryError(
                f"satellite {self.sat_id}: non-finite observation values")
        if self.pseudorange <= 0.0:
            raise InitialGeometryError(
                f"satellite {self.sat_id}: pseudorange must be positive")
        if self.sigma <= 0.0:
            raise InitialGeometryError(
                f"satellite {self.sat_id}: sigma must be positive")


@dataclass
class EpochSolution:
    x: float
    y: float
    z: float
    clock: float
    iterations: int
    residuals: dict[str, float]
    weighted_residual_sum_squares: float
    covariance: list[list[float]] = field(repr=False)

    @property
    def position(self) -> tuple[float, float, float]:
        return (self.x, self.y, self.z)


def _design_row(sat: SatelliteObs, state: list[float]) -> tuple[list[float], float]:
    dx = state[0] - sat.x
    dy = state[1] - sat.y
    dz = state[2] - sat.z
    rho = math.sqrt(dx * dx + dy * dy + dz * dz)
    if rho < MIN_RANGE_M:
        raise InitialGeometryError(
            f"satellite {sat.sat_id}: receiver state coincides with satellite")
    row = [dx / rho, dy / rho, dz / rho, 1.0]
    predicted = rho + state[3]
    return row, predicted


def solve_epoch(
    sats: list[SatelliteObs],
    initial: tuple[float, float, float, float],
    max_iterations: int = MAX_ITERATIONS,
    tol: float = CONVERGENCE_TOL_M,
) -> EpochSolution:
    """Estimate receiver position and common clock offset for one epoch.

    Weights are 1/sigma^2. Raises SolverError subclasses on degenerate
    geometry, invalid initial state, or non-convergence.
    """
    if len(sats) < 4:
        raise InitialGeometryError("at least four satellites are required")
    ids = [s.sat_id for s in sats]
    if len(set(ids)) != len(ids):
        raise InitialGeometryError("duplicate satellite identifiers in epoch")
    for sat in sats:
        sat.validate()
    if not is_finite_vector(list(initial)):
        raise InitialGeometryError("initial state contains non-finite values")

    state = [float(v) for v in initial]
    iterations = 0
    for iteration in range(1, max_iterations + 1):
        rows: list[list[float]] = []
        rhs: list[float] = []
        for sat in sats:
            row, predicted = _design_row(sat, state)
            weight = 1.0 / (sat.sigma * sat.sigma)
            rows.append([weight * v for v in row])
            rhs.append(weight * (sat.pseudorange - predicted))
        if matrix_rank(rows) < 4:
            raise RankDeficientError(
                "design matrix is rank deficient (degenerate geometry)")
        normal = [[0.0] * 4 for _ in range(4)]
        vec = [0.0] * 4
        for row, r in zip(rows, rhs):
            for i in range(4):
                vec[i] += row[i] * r
                for j in range(4):
                    normal[i][j] += row[i] * row[j]
        try:
            delta = solve_linear(normal, vec)
        except ValueError as exc:
            raise RankDeficientError(
                "normal matrix is singular (degenerate geometry)") from exc
        state = [s + d for s, d in zip(state, delta)]
        iterations = iteration
        if math.sqrt(sum(d * d for d in delta)) < tol:
            break
    else:
        raise ConvergenceError(
            f"least squares did not converge in {max_iterations} iterations")

    residuals: dict[str, float] = {}
    wss = 0.0
    normal = [[0.0] * 4 for _ in range(4)]
    for sat in sats:
        row, predicted = _design_row(sat, state)
        weight = 1.0 / (sat.sigma * sat.sigma)
        resid = sat.pseudorange - predicted
        residuals[sat.sat_id] = resid
        wss += weight * resid * resid
        for i in range(4):
            for j in range(4):
                normal[i][j] += weight * row[i] * row[j]
    try:
        covariance = invert(normal)
    except ValueError as exc:
        raise RankDeficientError(
            "final normal matrix is singular (degenerate geometry)") from exc
    return EpochSolution(
        x=state[0], y=state[1], z=state[2], clock=state[3],
        iterations=iterations, residuals=residuals,
        weighted_residual_sum_squares=wss, covariance=covariance,
    )
