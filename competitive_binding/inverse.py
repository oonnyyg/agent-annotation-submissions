"""Inverse problem: minimum total of ligand A to reach a target occupancy.

Ligand B total is fixed. Occupancy of ligand A on any site class is
monotonically non-decreasing in total A, so bisection on [0, limit] finds
the minimum total A meeting the target. If the target is not reached even
at the limit, the failure is reported explicitly - the limit is never
returned as if it were a solution.
"""

from __future__ import annotations

from dataclasses import dataclass

from .solver import solve_equilibrium, _check_finite


@dataclass
class InverseResult:
    success: bool
    target_class: str
    target_occupancy: float
    fixed_total_b: float
    limit_a: float
    total_a: float | None
    achieved_occupancy: float
    iterations: int
    message: str
    equilibrium: object | None


def solve_inverse(
    sites,
    target_class,
    target_occupancy,
    fixed_total_b,
    limit_a,
    tol=1e-10,
    occ_tol=1e-9,
    max_iter=200,
):
    """Find the minimum total A in [0, limit_a] reaching target occupancy."""
    names = [s.name for s in sites]
    if target_class not in names:
        raise ValueError(
            f"unknown target class {target_class!r}; available: {names}"
        )
    target_occupancy = _check_finite("target_occupancy", target_occupancy)
    if not (0.0 <= target_occupancy < 1.0):
        raise ValueError("target_occupancy must be in [0, 1)")
    fixed_total_b = _check_finite("fixed_total_b", fixed_total_b)
    if fixed_total_b < 0.0:
        raise ValueError("fixed_total_b must be non-negative")
    limit_a = _check_finite("limit_a", limit_a)
    if limit_a < 0.0:
        raise ValueError("limit_a must be non-negative")
    occ_tol = _check_finite("occ_tol", occ_tol)
    if occ_tol <= 0.0:
        raise ValueError("occ_tol must be positive")

    idx = names.index(target_class)

    def occupancy_at(total_a):
        res = solve_equilibrium(sites, total_a, fixed_total_b, tol=tol,
                                max_iter=max_iter)
        return res.sites[idx].occupancy_a, res

    if target_occupancy == 0.0:
        _, res = occupancy_at(0.0)
        return InverseResult(
            success=True,
            target_class=target_class,
            target_occupancy=target_occupancy,
            fixed_total_b=fixed_total_b,
            limit_a=limit_a,
            total_a=0.0,
            achieved_occupancy=res.sites[idx].occupancy_a,
            iterations=0,
            message="target occupancy is zero; no ligand A required",
            equilibrium=res,
        )

    occ_limit, res_limit = occupancy_at(limit_a)
    if occ_limit < target_occupancy - occ_tol:
        return InverseResult(
            success=False,
            target_class=target_class,
            target_occupancy=target_occupancy,
            fixed_total_b=fixed_total_b,
            limit_a=limit_a,
            total_a=None,
            achieved_occupancy=occ_limit,
            iterations=0,
            message=(
                f"target occupancy {target_occupancy:.6g} not reachable: "
                f"at the limit total_a={limit_a:.6g} nM the occupancy of "
                f"{target_class!r} is only {occ_limit:.6g}"
            ),
            equilibrium=None,
        )

    lo, hi = 0.0, limit_a
    best = res_limit
    iterations = 0
    for iterations in range(1, max_iter + 1):
        mid = 0.5 * (lo + hi)
        occ, res = occupancy_at(mid)
        if occ >= target_occupancy:
            hi = mid
            best = res
        else:
            lo = mid
        if hi - lo <= max(tol, occ_tol * max(hi, 1.0)):
            break
    final = solve_equilibrium(sites, hi, fixed_total_b, tol=tol,
                              max_iter=max_iter)
    return InverseResult(
        success=True,
        target_class=target_class,
        target_occupancy=target_occupancy,
        fixed_total_b=fixed_total_b,
        limit_a=limit_a,
        total_a=hi,
        achieved_occupancy=final.sites[idx].occupancy_a,
        iterations=iterations,
        message=(
            f"minimum total_a={hi:.6g} nM reaches occupancy "
            f"{final.sites[idx].occupancy_a:.6g} on {target_class!r}"
        ),
        equilibrium=final,
    )
