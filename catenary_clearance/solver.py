"""Exact catenary solver for a uniform, inextensible cable under self-weight.

Model: 2D statics, cable weight per unit length w = linear_density * g.
Shape: y(x) = a*cosh((x - xv)/a) + k, with H = w*a the horizontal tension.
"""

import math
from dataclasses import dataclass, field

_MAX_ITER = 200
_RTOL = 1e-13


class SolveError(Exception):
    """Raised when the catenary parameter cannot be determined."""


def _solve_a(span, slack, tol=_RTOL):
    """Solve 2*a*sinh(span/(2a)) = slack for a > 0 (slack > span > 0).

    f(a) = 2a*sinh(span/(2a)) is strictly decreasing from +inf (a->0+)
    to span (a->inf), so a unique root exists. Geometrically expanding
    bracket plus bisection stays robust both near-taut (a huge) and
    deeply sagged (a tiny).
    """
    def f(a):
        t = span / (2.0 * a)
        if t > 350.0:  # avoid overflow; f is huge positive here anyway
            return math.inf
        return 2.0 * a * math.sinh(t) - slack

    lo, hi = 1.0, 1.0
    tries = 0
    while f(lo) < 0.0:
        lo *= 0.5
        tries += 1
        if tries > _MAX_ITER:
            raise SolveError("could not bracket catenary parameter (lower bound)")
    tries = 0
    while f(hi) > 0.0:
        hi *= 2.0
        tries += 1
        if tries > _MAX_ITER:
            raise SolveError("could not bracket catenary parameter (upper bound)")
    for _ in range(_MAX_ITER):
        mid = 0.5 * (lo + hi)
        fm = f(mid)
        if fm == 0.0 or (hi - lo) <= tol * max(1.0, mid):
            return mid
        if fm > 0.0:
            lo = mid
        else:
            hi = mid
    raise SolveError("catenary parameter iteration did not converge")


@dataclass
class CatenarySolution:
    x0: float
    y0: float
    x1: float
    y1: float
    length: float
    weight_per_length: float
    a: float = field(init=False)
    xv: float = field(init=False)
    k: float = field(init=False)

    def __post_init__(self):
        span = self.x1 - self.x0
        dy = self.y1 - self.y0
        chord = math.hypot(span, dy)
        if not self.length > chord:
            raise SolveError(
                f"cable length {self.length} not greater than chord {chord}"
            )
        slack = math.sqrt(self.length * self.length - dy * dy)
        self.a = _solve_a(span, slack)
        ratio = dy / slack
        self.xv = self.x0 + 0.5 * span - self.a * math.asinh(ratio)
        self.k = self.y0 - self.a * math.cosh((self.x0 - self.xv) / self.a)

    def y(self, x):
        return self.a * math.cosh((x - self.xv) / self.a) + self.k

    def slope(self, x):
        return math.sinh((x - self.xv) / self.a)

    def arc_length(self, xa, xb):
        return self.a * (math.sinh((xb - self.xv) / self.a)
                         - math.sinh((xa - self.xv) / self.a))

    @property
    def horizontal_tension(self):
        return self.weight_per_length * self.a

    def tension(self, x):
        return self.horizontal_tension * math.cosh((x - self.xv) / self.a)

    @property
    def vertex(self):
        return (self.xv, self.k + self.a)

    def lowest_point_in_span(self):
        x_lo = min(max(self.xv, self.x0), self.x1)
        return (x_lo, self.y(x_lo))

    def residuals(self):
        return {
            "endpoint_y": abs(self.y(self.x1) - self.y1),
            "arc_length": abs(self.arc_length(self.x0, self.x1) - self.length),
        }
