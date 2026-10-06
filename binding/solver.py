"""Coupled equilibrium solver for two ligands competing for finite sites.

For each site class i with free concentrations (fa, fb):
    Z_i = 1 + fa/Kd_ia + fb/Kd_ib
    p_free = 1/Z_i, p_a = (fa/Kd_ia)/Z_i, p_b = (fb/Kd_ib)/Z_i
Mass conservation is solved simultaneously across all classes:
    R_a = fa + sum_i S_i * p_a_i - T_a = 0
    R_b = fb + sum_i S_i * p_b_i - T_b = 0
Solved by damped Newton iteration with the analytic 2x2 Jacobian.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field
from typing import Sequence

from .model import Experiment, SiteClass

MAX_ITERATIONS = 100
RTOL = 1e-12
ATOL = 1e-12  # nM


class ConvergenceError(RuntimeError):
    """Raised when the Newton iteration fails to converge."""

    def __init__(self, message: str, residuals: tuple, iterations: int):
        super().__init__(message)
        self.residuals = residuals
        self.iterations = iterations


@dataclass(frozen=True)
class ClassResult:
    name: str
    total: float
    frac_free: float
    occ_a: float
    occ_b: float
    bound_a: float
    bound_b: float


@dataclass(frozen=True)
class EquilibriumResult:
    experiment: Experiment
    free_a: float
    free_b: float
    classes: tuple
    residual_a: float
    residual_b: float
    iterations: int
    converged: bool = field(default=True)

    @property
    def bound_a(self) -> float:
        return sum(c.bound_a for c in self.classes)

    @property
    def bound_b(self) -> float:
        return sum(c.bound_b for c in self.classes)


def _residuals_and_jacobian(sites, fa, fb, ta, tb):
    ra, rb = fa - ta, fb - tb
    jaa, jab, jba, jbb = 1.0, 0.0, 0.0, 1.0
    for s in sites:
        ua, ub = fa / s.kd_a, fb / s.kd_b
        z = 1.0 + ua + ub
        z2 = z * z
        ra += s.total * ua / z
        rb += s.total * ub / z
        jaa += s.total * (1.0 + ub) / (s.kd_a * z2)
        jab -= s.total * ua / (s.kd_b * z2)
        jba -= s.total * ub / (s.kd_a * z2)
        jbb += s.total * (1.0 + ua) / (s.kd_b * z2)
    return (ra, rb), ((jaa, jab), (jba, jbb))


def _tolerance(total: float) -> float:
    return ATOL + RTOL * total


def solve_equilibrium(sites, experiment: Experiment) -> EquilibriumResult:
    if not sites:
        raise ValueError("at least one site class is required")
    ta, tb = experiment.total_a, experiment.total_b
    fa, fb = ta, tb
    ra = rb = math.inf
    iterations = 0
    for iterations in range(1, MAX_ITERATIONS + 1):
        (ra, rb), ((jaa, jab), (jba, jbb)) = _residuals_and_jacobian(
            sites, fa, fb, ta, tb
        )
        if abs(ra) <= _tolerance(ta) and abs(rb) <= _tolerance(tb):
            break
        det = jaa * jbb - jab * jba
        if det <= 0.0 or not math.isfinite(det):
            raise ConvergenceError(
                f"singular Jacobian (det={det!r})", (ra, rb), iterations
            )
        dfa = (jbb * ra - jab * rb) / det
        dfb = (-jba * ra + jaa * rb) / det
        step = 1.0
        while fa - step * dfa < 0.0 or fb - step * dfb < 0.0:
            step *= 0.5
            if step < 1e-12:
                raise ConvergenceError(
                    "line search underflow", (ra, rb), iterations
                )
        fa -= step * dfa
        fb -= step * dfb
    else:
        raise ConvergenceError(
            f"not converged after {MAX_ITERATIONS} iterations "
            f"(residuals: R_a={ra:.6e} nM, R_b={rb:.6e} nM)",
            (ra, rb),
            iterations,
        )
    fa = min(max(fa, 0.0), ta)
    fb = min(max(fb, 0.0), tb)
    (ra, rb), _ = _residuals_and_jacobian(sites, fa, fb, ta, tb)
    if abs(ra) > _tolerance(ta) or abs(rb) > _tolerance(tb):
        raise ConvergenceError(
            f"final residuals exceed tolerance (R_a={ra:.6e}, R_b={rb:.6e} nM)",
            (ra, rb),
            iterations,
        )
    classes = tuple(_class_result(s, fa, fb) for s in sites)
    return EquilibriumResult(
        experiment=experiment,
        free_a=fa,
        free_b=fb,
        classes=classes,
        residual_a=ra,
        residual_b=rb,
        iterations=iterations,
    )


def _class_result(site: SiteClass, fa: float, fb: float) -> ClassResult:
    ua, ub = fa / site.kd_a, fb / site.kd_b
    z = 1.0 + ua + ub
    return ClassResult(
        name=site.name,
        total=site.total,
        frac_free=1.0 / z,
        occ_a=ua / z,
        occ_b=ub / z,
        bound_a=site.total * ua / z,
        bound_b=site.total * ub / z,
    )
