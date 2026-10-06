"""Single-stage flash solved with the Rachford-Rice equation."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

TOLERANCE = 1e-12
MAX_ITERATIONS = 200


class FlashError(ValueError):
    """Raised when a flash problem is invalid or fails to converge."""


@dataclass
class FlashResult:
    phase: str  # "liquid", "vapor", "two_phase"
    vapor_fraction: float
    liquid_flows: dict[str, float]
    vapor_flows: dict[str, float]
    residual: float
    iterations: int
    degenerate: bool = False
    warnings: list[str] = field(default_factory=list)


def _rr(vapor_fraction: float, z: list[float], k: list[float]) -> float:
    return sum(zi * (ki - 1.0) / (1.0 + vapor_fraction * (ki - 1.0)) for zi, ki in zip(z, k))


def solve_flash(feed: dict[str, float], k_values: dict[str, float]) -> FlashResult:
    """Flash a feed (component molar flows) with given K-values."""
    names = list(feed)
    flows = []
    for name in names:
        value = float(feed[name])
        if not math.isfinite(value) or value < 0.0:
            raise FlashError(f"feed flow for {name!r} must be a finite non-negative number")
        flows.append(value)
    total = sum(flows)
    if total <= 0.0:
        raise FlashError("total feed flow is zero; nothing to flash")

    k = []
    for name in names:
        if name not in k_values:
            raise FlashError(f"missing K-value for component {name!r}")
        ki = float(k_values[name])
        if not math.isfinite(ki) or ki <= 0.0:
            raise FlashError(f"K-value for {name!r} must be a positive finite number")
        k.append(ki)

    z = [f / total for f in flows]
    active = [i for i, zi in enumerate(z) if zi > 0.0]

    warnings: list[str] = []
    if all(k[i] == 1.0 for i in active):
        # Degenerate: every present component has K == 1; treat as all liquid.
        return FlashResult(
            phase="liquid",
            vapor_fraction=0.0,
            liquid_flows=dict(zip(names, flows)),
            vapor_flows={n: 0.0 for n in names},
            residual=0.0,
            iterations=0,
            degenerate=True,
            warnings=["all K-values are 1.0; phase split is indeterminate, assumed all liquid"],
        )

    rr0 = _rr(0.0, z, k)
    rr1 = _rr(1.0, z, k)
    if rr0 <= 0.0:
        return FlashResult("liquid", 0.0, dict(zip(names, flows)),
                           {n: 0.0 for n in names}, rr0, 0, False, warnings)
    if rr1 >= 0.0:
        return FlashResult("vapor", 1.0, {n: 0.0 for n in names},
                           dict(zip(names, flows)), rr1, 0, False, warnings)

    # Bisection on (0, 1); f is strictly monotone, endpoints bracket the root.
    lo, hi = 0.0, 1.0
    flo, fhi = rr0, rr1
    mid = 0.5
    fmid = _rr(mid, z, k)
    iterations = 0
    for iterations in range(1, MAX_ITERATIONS + 1):
        mid = 0.5 * (lo + hi)
        fmid = _rr(mid, z, k)
        if abs(fmid) <= TOLERANCE or (hi - lo) <= TOLERANCE:
            break
        if (flo < 0.0) == (fmid < 0.0):
            lo, flo = mid, fmid
        else:
            hi, fhi = mid, fmid
    else:
        raise FlashError("Rachford-Rice iteration did not converge")

    vapor_fraction = mid
    liquid_flows = {}
    vapor_flows = {}
    for name, f, ki in zip(names, flows, k):
        li = f * (1.0 - vapor_fraction) / (1.0 + vapor_fraction * (ki - 1.0))
        liquid_flows[name] = li
        vapor_flows[name] = f - li
    return FlashResult("two_phase", vapor_fraction, liquid_flows, vapor_flows,
                       abs(fmid), iterations, False, warnings)
