"""Leave-one-out sensitivity analysis for epochs with >= 5 satellites."""

from __future__ import annotations

import math
from dataclasses import dataclass

from .solver import EpochSolution, SatelliteObs, SolverError, solve_epoch


@dataclass
class LeaveOneOutResult:
    excluded_sat_id: str
    ok: bool
    error: str | None = None
    position_shift_m: float | None = None
    clock_shift_m: float | None = None
    extrapolated_residual_m: float | None = None


def leave_one_out(
    sats: list[SatelliteObs],
    initial: tuple[float, float, float, float],
    full: EpochSolution,
) -> list[LeaveOneOutResult]:
    """Re-solve the epoch dropping one satellite at a time.

    Each subset solve restarts from the epoch's original initial state.
    Degenerate subsets are flagged, not treated as the correct answer.
    """
    results: list[LeaveOneOutResult] = []
    if len(sats) < 5:
        return results
    for excluded in sats:
        subset = [s for s in sats if s.sat_id != excluded.sat_id]
        try:
            sub = solve_epoch(subset, initial)
        except SolverError as exc:
            results.append(LeaveOneOutResult(
                excluded_sat_id=excluded.sat_id, ok=False,
                error=f"{type(exc).__name__}: {exc}"))
            continue
        shift = math.sqrt(
            (sub.x - full.x) ** 2 + (sub.y - full.y) ** 2
            + (sub.z - full.z) ** 2)
        dx = sub.x - excluded.x
        dy = sub.y - excluded.y
        dz = sub.z - excluded.z
        rho = math.sqrt(dx * dx + dy * dy + dz * dz)
        extrapolated = excluded.pseudorange - (rho + sub.clock)
        results.append(LeaveOneOutResult(
            excluded_sat_id=excluded.sat_id, ok=True,
            position_shift_m=shift,
            clock_shift_m=sub.clock - full.clock,
            extrapolated_residual_m=extrapolated))
    return results
