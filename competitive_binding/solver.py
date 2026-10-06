"""Equilibrium solver for two ligands competing for independent site classes.

Model (1:1 binding, mutually exclusive occupancy per site):
    For site class i with total S_i and dissociation constants Ka_i, Kb_i,
    and free ligand concentrations a, b:
        denom_i = 1 + a/Ka_i + b/Kb_i
        empty_i = S_i / denom_i
        boundA_i = S_i * (a/Ka_i) / denom_i
        boundB_i = S_i * (b/Kb_i) / denom_i
    Mass conservation:
        A_tot = a + sum_i boundA_i
        B_tot = b + sum_i boundB_i
Free concentrations are solved jointly; site classes are never solved
independently and summed afterwards.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field


class SolverError(Exception):
    """Raised when the equilibrium solve fails to converge."""

    def __init__(self, message, residual_a=None, residual_b=None):
        super().__init__(message)
        self.residual_a = residual_a
        self.residual_b = residual_b


def _check_finite(name, value):
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ValueError(f"{name} must be a number, got {value!r}")
    if not math.isfinite(value):
        raise ValueError(f"{name} must be finite, got {value!r}")
    return float(value)


@dataclass
class SiteClass:
    """One independent class of binding sites (concentrations in nM)."""

    name: str
    total: float
    kd_a: float
    kd_b: float

    def __post_init__(self):
        self.total = _check_finite(f"site {self.name!r} total", self.total)
        self.kd_a = _check_finite(f"site {self.name!r} kd_a", self.kd_a)
        self.kd_b = _check_finite(f"site {self.name!r} kd_b", self.kd_b)
        if self.total <= 0.0:
            raise ValueError(f"site {self.name!r} total must be positive")
        if self.kd_a <= 0.0 or self.kd_b <= 0.0:
            raise ValueError(f"site {self.name!r} Kd values must be positive")

    def states(self, free_a, free_b):
        """Return (empty, bound_a, bound_b) concentrations for this class."""
        wa = free_a / self.kd_a
        wb = free_b / self.kd_b
        denom = 1.0 + wa + wb
        return self.total / denom, self.total * wa / denom, self.total * wb / denom


@dataclass
class SiteResult:
    name: str
    total: float
    empty: float
    bound_a: float
    bound_b: float
    occupancy_a: float
    occupancy_b: float


@dataclass
class MixtureResult:
    total_a: float
    total_b: float
    free_a: float
    free_b: float
    residual_a: float
    residual_b: float
    iterations: int
    sites: list = field(default_factory=list)


def _residuals(sites, free_a, free_b, total_a, total_b):
    bound_a = 0.0
    bound_b = 0.0
    for s in sites:
        _, ba, bb = s.states(free_a, free_b)
        bound_a += ba
        bound_b += bb
    return free_a + bound_a - total_a, free_b + bound_b - total_b


def _solve_a_for_b(sites, free_b, total_a, tol, max_iter):
    """1-D bisection: free a in [0, total_a] given fixed free b."""
    if total_a == 0.0:
        return 0.0
    lo, hi = 0.0, total_a
    for _ in range(max_iter):
        mid = 0.5 * (lo + hi)
        ra, _ = _residuals(sites, mid, free_b, total_a, 0.0)
        if abs(ra) <= tol:
            return mid
        if ra > 0.0:
            hi = mid
        else:
            lo = mid
    return 0.5 * (lo + hi)


def solve_equilibrium(sites, total_a, total_b, tol=1e-10, max_iter=200):
    """Solve joint mass conservation for free (a, b).

    Nested monotone bisection: outer loop brackets free b in [0, total_b],
    inner loop solves free a in [0, total_a] for that b. Both residual
    functions are strictly monotone in their unknown, so the bracket is
    guaranteed to contain the unique root (zero totals included).
    """
    if not sites:
        raise ValueError("at least one site class is required")
    total_a = _check_finite("total_a", total_a)
    total_b = _check_finite("total_b", total_b)
    if total_a < 0.0 or total_b < 0.0:
        raise ValueError("total ligand concentrations must be non-negative")
    tol = _check_finite("tol", tol)
    if tol <= 0.0:
        raise ValueError("tol must be positive")

    if total_b == 0.0:
        free_b = 0.0
        free_a = _solve_a_for_b(sites, 0.0, total_a, tol, max_iter)
        iterations = 1
    else:
        lo, hi = 0.0, total_b
        free_b = None
        free_a = 0.0
        iterations = 0
        for iterations in range(1, max_iter + 1):
            mid = 0.5 * (lo + hi)
            free_a = _solve_a_for_b(sites, mid, total_a, tol, max_iter)
            _, rb = _residuals(sites, free_a, mid, total_a, total_b)
            if abs(rb) <= tol:
                free_b = mid
                break
            if rb > 0.0:
                hi = mid
            else:
                lo = mid
            free_b = 0.5 * (lo + hi)
        else:
            ra, rb = _residuals(sites, free_a, free_b, total_a, total_b)
            raise SolverError(
                f"equilibrium did not converge after {max_iter} iterations "
                f"(residual_a={ra:.6g} nM, residual_b={rb:.6g} nM)",
                residual_a=ra,
                residual_b=rb,
            )

    ra, rb = _residuals(sites, free_a, free_b, total_a, total_b)
    scale = max(total_a, total_b, 1.0)
    if abs(ra) > 100 * tol * scale or abs(rb) > 100 * tol * scale:
        raise SolverError(
            f"equilibrium residuals exceed tolerance "
            f"(residual_a={ra:.6g} nM, residual_b={rb:.6g} nM)",
            residual_a=ra,
            residual_b=rb,
        )
    if not (0.0 <= free_a <= total_a) or not (0.0 <= free_b <= total_b):
        raise SolverError(
            f"free concentrations out of bounds (free_a={free_a:.6g}, "
            f"free_b={free_b:.6g})",
            residual_a=ra,
            residual_b=rb,
        )

    site_results = []
    for s in sites:
        empty, ba, bb = s.states(free_a, free_b)
        site_results.append(
            SiteResult(
                name=s.name,
                total=s.total,
                empty=empty,
                bound_a=ba,
                bound_b=bb,
                occupancy_a=ba / s.total,
                occupancy_b=bb / s.total,
            )
        )
    return MixtureResult(
        total_a=total_a,
        total_b=total_b,
        free_a=free_a,
        free_b=free_b,
        residual_a=ra,
        residual_b=rb,
        iterations=iterations,
        sites=site_results,
    )
