"""Exact catenary solver for a uniform, inextensible cable between two supports.

Model: 2D static self-weight only. The cable shape is

    y(x) = y_vertex + a * (cosh((x - x_vertex) / a) - 1)

where ``a = H / w`` with H the horizontal tension and ``w`` the weight per
unit length (linear density times gravity).
"""

from __future__ import annotations

import math
from dataclasses import dataclass

# Tolerances for the numerical solves and residual checks.
TOL_NEWTON = 1e-12
TOL_RESIDUAL_REL = 1e-9
MAX_ITER = 200
# sinh(t)/t is evaluated by series below this |t| to avoid cancellation.
_SERIES_CUTOFF = 1e-3


class CatenaryError(Exception):
    """Raised when the catenary cannot be solved for the given inputs."""


def _sinh_over_t(t: float) -> float:
    """Return sinh(t)/t, accurate for t near zero."""
    if abs(t) < _SERIES_CUTOFF:
        t2 = t * t
        return 1.0 + t2 / 6.0 + t2 * t2 / 120.0 + t2 * t2 * t2 / 5040.0
    return math.sinh(t) / t


def _solve_sinh_ratio(ratio: float) -> float:
    """Solve sinh(t)/t = ratio for t >= 0 (ratio >= 1 required)."""
    if ratio < 1.0:
        raise CatenaryError(
            f"cable length shorter than chord (sinh(t)/t target {ratio!r} < 1)"
        )
    if ratio == 1.0:
        return 0.0
    # Initial guess: asymptotic for large ratio, series inversion for small.
    if ratio > 3.0:
        t = math.log(2.0 * ratio)
    else:
        t = math.sqrt(6.0 * (ratio - 1.0))
    for _ in range(MAX_ITER):
        f = _sinh_over_t(t) - ratio
        # Derivative of sinh(t)/t.
        if abs(t) < _SERIES_CUTOFF:
            df = t / 3.0 + t**3 / 30.0
        else:
            df = (t * math.cosh(t) - math.sinh(t)) / (t * t)
        if df <= 0.0:
            raise CatenaryError("non-positive derivative in catenary solve")
        step = f / df
        t -= step
        if t < 0.0:
            t = 0.0
        if abs(step) <= TOL_NEWTON * max(1.0, t):
            return t
    raise CatenaryError(
        f"catenary parameter solve did not converge after {MAX_ITER} iterations"
    )


@dataclass(frozen=True)
class Catenary:
    """A solved catenary between two supports."""

    x1: float
    y1: float
    x2: float
    y2: float
    length: float
    a: float  # H / w
    x_vertex: float
    y_vertex: float
    weight_per_length: float  # linear density * gravity

    def y(self, x: float) -> float:
        return self.y_vertex + self.a * (math.cosh((x - self.x_vertex) / self.a) - 1.0)

    def slope(self, x: float) -> float:
        return math.sinh((x - self.x_vertex) / self.a)

    @property
    def horizontal_tension(self) -> float:
        return self.weight_per_length * self.a

    def tension_at(self, x: float) -> float:
        return self.horizontal_tension * math.cosh((x - self.x_vertex) / self.a)

    @property
    def support_tensions(self) -> tuple[float, float]:
        return self.tension_at(self.x1), self.tension_at(self.x2)

    @property
    def max_tension(self) -> float:
        return max(self.support_tensions)

    def span_lowest_point(self) -> tuple[float, float]:
        """Lowest point restricted to [x1, x2] (vertex may lie outside)."""
        x = min(max(self.x_vertex, self.x1), self.x2)
        return x, self.y(x)

    def residuals(self) -> dict[str, float]:
        """Endpoint and arc-length residuals of the solved shape."""
        e1 = abs(self.y(self.x1) - self.y1)
        e2 = abs(self.y(self.x2) - self.y2)
        # Analytic arc length between the supports.
        arc = self.a * (
            math.sinh((self.x2 - self.x_vertex) / self.a)
            - math.sinh((self.x1 - self.x_vertex) / self.a)
        )
        return {
            "endpoint_left": e1,
            "endpoint_right": e2,
            "arc_length": abs(arc - self.length),
        }


def solve_catenary(
    x1: float,
    y1: float,
    x2: float,
    y2: float,
    length: float,
    weight_per_length: float,
) -> Catenary:
    """Solve the catenary through (x1, y1) and (x2, y2) with arc length ``length``.

    Requires x2 > x1, length strictly greater than the chord, and a positive
    weight per unit length.
    """
    if not x2 > x1:
        raise CatenaryError("supports must satisfy x2 > x1")
    if weight_per_length <= 0.0:
        raise CatenaryError("weight per unit length must be positive")
    dx = x2 - x1
    dy = y2 - y1
    chord = math.hypot(dx, dy)
    if not length > chord:
        raise CatenaryError(
            f"cable length {length!r} must exceed chord length {chord!r}"
        )
    slack = math.sqrt(length * length - dy * dy)
    t = _solve_sinh_ratio(slack / dx)
    if t == 0.0:
        raise CatenaryError("degenerate (taut) cable; length must exceed chord")
    a = dx / (2.0 * t)
    # Locate the vertex from the endpoint height difference.
    sinh_t = math.sinh(t)
    x_vertex = 0.5 * (x1 + x2) - a * math.asinh(dy / (2.0 * a * sinh_t))
    y_vertex = y1 - a * (math.cosh((x1 - x_vertex) / a) - 1.0)
    cat = Catenary(
        x1=x1,
        y1=y1,
        x2=x2,
        y2=y2,
        length=length,
        a=a,
        x_vertex=x_vertex,
        y_vertex=y_vertex,
        weight_per_length=weight_per_length,
    )
    res = cat.residuals()
    scale = max(1.0, abs(y1), abs(y2), length)
    if (
        res["endpoint_left"] > TOL_RESIDUAL_REL * scale
        or res["endpoint_right"] > TOL_RESIDUAL_REL * scale
        or res["arc_length"] > TOL_RESIDUAL_REL * scale
    ):
        raise CatenaryError(f"solved catenary failed residual check: {res}")
    return cat
