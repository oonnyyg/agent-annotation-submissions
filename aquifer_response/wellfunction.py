"""Numerical evaluation of the Theis well function W(u) = E1(u).

W(u) is the exponential integral E1(u) = integral_u^inf exp(-t)/t dt.
Two complementary algorithms are used so that both very small and very
large dimensionless time parameters u are handled accurately:

* Power series (Abramowitz & Stegun 5.1.11) for u <= 1.
* Lentz continued fraction (A&S 5.1.22) for u > 1.

The evaluation is a pure function of (u, tolerance, max_terms): repeated
queries with the same arguments always return identical results.
"""

from __future__ import annotations

import math
from dataclasses import dataclass

EULER_MASCHERONI = 0.5772156649015328606

DEFAULT_TOLERANCE = 1e-14
DEFAULT_MAX_TERMS = 10000


class WellFunctionError(Exception):
    """Raised when W(u) cannot be evaluated to the requested accuracy."""

    def __init__(self, reason: str, u: float):
        self.reason = reason
        self.u = u
        super().__init__(f"W(u) evaluation failed for u={u!r}: {reason}")


@dataclass(frozen=True)
class WellFunctionResult:
    u: float
    value: float
    method: str
    terms: int
    tolerance: float


def _series_terms(u: float, tol: float, max_terms: int) -> WellFunctionResult:
    total = -EULER_MASCHERONI - math.log(u)
    # term_k = (-u)^k / (k * k!)
    power = 1.0  # (-u)^k
    factorial = 1.0  # k!
    for k in range(1, max_terms + 1):
        power *= -u
        factorial *= k
        term = power / (k * factorial)
        total -= term
        if abs(term) <= tol * max(1.0, abs(total)):
            return WellFunctionResult(u, total, "series", k, tol)
    raise WellFunctionError(
        f"power series did not converge within {max_terms} terms", u
    )


def _continued_fraction(u: float, tol: float, max_terms: int) -> WellFunctionResult:
    # E1(u) = exp(-u) * CF, CF = 1/(u+ 1/(1+ 1/(u+ 2/(1+ 2/(u+ ...))))))
    # evaluated with the modified Lentz algorithm.
    tiny = 1e-300
    f = u if abs(u) > tiny else tiny
    c = f
    d = 0.0
    for n in range(1, max_terms + 1):
        # A&S 5.1.22: E1(x) = e^-x * 1/(x+ 1/(1+ 1/(x+ 2/(1+ 2/(x+ ...
        # numerators a_n = 1, 1, 2, 2, 3, 3, ... ; denominators alternate 1, x
        a = float((n + 1) // 2)
        b = 1.0 if n % 2 == 1 else u
        d = b + a * d
        if abs(d) < tiny:
            d = tiny
        c = b + a / c
        if abs(c) < tiny:
            c = tiny
        d = 1.0 / d
        delta = c * d
        f *= delta
        if abs(delta - 1.0) <= tol:
            # f converges to the denominator u + 1/(1+ 1/(u+ ...));
            # E1(u) = exp(-u) divided by that denominator.
            value = math.exp(-u) / f
            return WellFunctionResult(u, value, "continued_fraction", n, tol)
    raise WellFunctionError(
        f"continued fraction did not converge within {max_terms} terms", u
    )


def well_function(
    u: float,
    tolerance: float = DEFAULT_TOLERANCE,
    max_terms: int = DEFAULT_MAX_TERMS,
) -> WellFunctionResult:
    """Evaluate W(u) for u > 0 with explicit accuracy control."""
    if not math.isfinite(u) or u <= 0.0:
        raise WellFunctionError("u must be a positive finite number", u)
    if tolerance <= 0.0 or not math.isfinite(tolerance):
        raise WellFunctionError("tolerance must be a positive finite number", u)
    if max_terms < 1:
        raise WellFunctionError("max_terms must be >= 1", u)
    if u <= 1.0:
        return _series_terms(u, tolerance, max_terms)
    return _continued_fraction(u, tolerance, max_terms)


def W(u: float, tolerance: float = DEFAULT_TOLERANCE) -> float:
    """Convenience wrapper returning only the value of W(u)."""
    return well_function(u, tolerance).value
