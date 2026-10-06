"""Inverse problem: minimal total of ligand A to reach a target occupancy.

With total_b fixed, occupancy of ligand A on any class is monotonically
non-decreasing in total_a, so bisection on [0, limit_a] finds the minimal
dose. If occupancy at the limit is still below target, the target is
reported as unreachable; the limit is never returned as a solution.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Optional, Sequence

from .model import Experiment, SiteClass
from .solver import EquilibriumResult, solve_equilibrium

DOSE_RTOL = 1e-10
DOSE_ATOL = 1e-9  # nM
MAX_BISECTION = 200


@dataclass(frozen=True)
class InverseResult:
    target_class: str
    target_occupancy: float
    total_b: float
    limit_a: float
    achieved: bool
    total_a: Optional[float]
    occupancy_at_solution: Optional[float]
    occupancy_at_limit: float
    equilibrium: Optional[EquilibriumResult]
    message: str


def _occupancy(sites: Sequence[SiteClass], index: int, ta: float, tb: float) -> float:
    return solve_equilibrium(sites, Experiment(ta, tb)).classes[index].occ_a


def invert_dose(
    sites: Sequence[SiteClass],
    target_class: str,
    target_occupancy: float,
    total_b: float,
    limit_a: float,
) -> InverseResult:
    names = [s.name for s in sites]
    if target_class not in names:
        raise ValueError(f"unknown site class {target_class!r}; have {names}")
    index = names.index(target_class)
    if not (0.0 <= target_occupancy < 1.0):
        raise ValueError(
            f"target occupancy must be in [0, 1), got {target_occupancy}"
        )
    if total_b < 0.0 or limit_a < 0.0:
        raise ValueError("total_b and limit_a must be non-negative")

    occ_limit = _occupancy(sites, index, limit_a, total_b)
    if occ_limit < target_occupancy:
        return InverseResult(
            target_class=target_class,
            target_occupancy=target_occupancy,
            total_b=total_b,
            limit_a=limit_a,
            achieved=False,
            total_a=None,
            occupancy_at_solution=None,
            occupancy_at_limit=occ_limit,
            equilibrium=None,
            message=(
                f"unreachable: occupancy at limit {limit_a} nM is "
                f"{occ_limit:.6g} < target {target_occupancy:.6g}"
            ),
        )
    if target_occupancy == 0.0:
        eq = solve_equilibrium(sites, Experiment(0.0, total_b))
        return InverseResult(
            target_class=target_class,
            target_occupancy=target_occupancy,
            total_b=total_b,
            limit_a=limit_a,
            achieved=True,
            total_a=0.0,
            occupancy_at_solution=eq.classes[index].occ_a,
            occupancy_at_limit=occ_limit,
            equilibrium=eq,
            message="target 0 reached with zero dose of ligand A",
        )

    lo, hi = 0.0, limit_a
    for _ in range(MAX_BISECTION):
        mid = 0.5 * (lo + hi)
        if _occupancy(sites, index, mid, total_b) >= target_occupancy:
            hi = mid
        else:
            lo = mid
        if hi - lo <= DOSE_ATOL + DOSE_RTOL * hi:
            break
    eq = solve_equilibrium(sites, Experiment(hi, total_b))
    return InverseResult(
        target_class=target_class,
        target_occupancy=target_occupancy,
        total_b=total_b,
        limit_a=limit_a,
        achieved=True,
        total_a=hi,
        occupancy_at_solution=eq.classes[index].occ_a,
        occupancy_at_limit=occ_limit,
        equilibrium=eq,
        message=f"minimal total_a ~= {hi:.6g} nM",
    )
