
"""Theis well function W(u) = E1(u) with accuracy control.

Two regimes are used so that both very small and very large values of the
dimensionless time parameter u are handled:

- Small u: the absolutely convergent power series
  W(u) = -gamma - ln(u) - sum_{k=1..inf} (-u)^k / (k * k!)
- Large u: the convergent continued fraction
  W(u) = exp(-u) / (u+ 1/(1+ 1/(u+ 2/(1+ 2/(u+ ...)))))
  evaluated with the modified Lentz algorithm, which converges to
  machine precision for any u > 1.
"""

from __future__ import annotations

import math

_EULER_GAMMA = 0.5772156649015328606
_SERIES_SWITCH_U = 1.0
# Beyond this u, W(u) underflows double precision; the value is effectively
# zero (exp(-745) is below the smallest subnormal).
_UNDERFLOW_U = 745.0


class WellFunctionError(Exception):
    """Raised when the well function cannot be evaluated accurately.

    The message always states the reason for the failure.
    """


def theis_w(u: float, tol: float = 1e-14, max_terms: int = 10000) -> float:
    """Evaluate the Theis well function W(u) for u > 0.

    u is the dimensionless time parameter u = r^2 S / (4 T t).
    Raises WellFunctionError with the failure reason on invalid input or
    non-convergence.
    """
    if not math.isfinite(u) or u <= 0.0:
        raise WellFunctionError(f"u must be a positive finite number, got {u!r}")
    if tol <= 0.0 or not math.isfinite(tol):
        raise WellFunctionError(f"tolerance must be positive, got {tol!r}")
    if max_terms < 1:
        raise WellFunctionError(f"max_terms must be >= 1, got {max_terms!r}")

    if u >= _UNDERFLOW_U:
        return 0.0
    if u <= _SERIES_SWITCH_U:
        return _series(u, tol, max_terms)
    return _asymptotic(u, tol, max_terms)


def _series(u: float, tol: float, max_terms: int) -> float:
    # W(u) = -gamma - ln(u) - sum_{k>=1} (-u)^k / (k * k!)
    total = -_EULER_GAMMA - math.log(u)
    term = 1.0
    for k in range(1, max_terms + 1):
        term *= -u / k
        add = term / k
        total -= add
        if abs(add) <= tol * max(1.0, abs(total)):
            return total
    raise WellFunctionError(
        f"power series for W({u!r}) did not converge within {max_terms} terms"
    )


def _asymptotic(u: float, tol: float, max_terms: int) -> float:
    # Modified Lentz algorithm for the convergent continued fraction
    #   W(u) = exp(-u) * 1/(u+1- 1^2/(u+3- 2^2/(u+5- 3^2/(u+7- ...))))
    tiny = 1e-300
    b = u + 1.0
    c = 1.0 / tiny
    d = 1.0 / b
    f = d
    for k in range(1, max_terms + 1):
        a = -(k * k)
        b += 2.0
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
            return math.exp(-u) * f
    raise WellFunctionError(
        f"continued fraction for W({u!r}) did not converge within "
        f"{max_terms} iterations"
    )
