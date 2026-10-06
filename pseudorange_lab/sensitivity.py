"""Leave-one-out sensitivity analysis for epochs with >= 5 satellites.

Each subset is re-solved from the epoch's original initial values.
Degenerate subsets are flagged, never silently substituted.
"""
from __future__ import annotations

import math
from dataclasses import dataclass

from .data import Observation
from .errors import ConvergenceError, GeometryError
from .solver import Solution, solve_epoch


@dataclass
class LeaveOneOutResult:
    excluded_sat_id: str
    status: str  # "ok" or "degenerate"
    detail: str | None
    position_shift_m: float | None
    clock_shift_m: float | None
    extrapolated_residual_m: float | None
    solution: Solution | None


def leave_one_out(
    observations: list[Observation],
    initial_position_m: tuple[float, float, float],
    initial_clock_m: float,
    full_solution: Solution,
) -> list[LeaveOneOutResult] | None:
    """Drop each satellite in turn and re-solve; None if fewer than 5 sats."""
    if len(observations) < 5:
        return None
    results: list[LeaveOneOutResult] = []
    for dropped in observations:
        subset = [o for o in observations if o.sat_id != dropped.sat_id]
        try:
            sol = solve_epoch(subset, initial_position_m, initial_clock_m)
        except (GeometryError, ConvergenceError) as exc:
            results.append(LeaveOneOutResult(
                excluded_sat_id=dropped.sat_id,
                status="degenerate",
                detail=str(exc),
                position_shift_m=None,
                clock_shift_m=None,
                extrapolated_residual_m=None,
                solution=None,
            ))
            continue
        shift_xyz = math.dist(sol.position_m, full_solution.position_m)
        shift_clk = abs(sol.clock_m - full_solution.clock_m)
        rng = math.dist(dropped.position_m, sol.position_m)
        extrapolated = dropped.pseudorange_m - (rng + sol.clock_m)
        results.append(LeaveOneOutResult(
            excluded_sat_id=dropped.sat_id,
            status="ok",
            detail=None,
            position_shift_m=shift_xyz,
            clock_shift_m=shift_clk,
            extrapolated_residual_m=extrapolated,
            solution=sol,
        ))
    return results
