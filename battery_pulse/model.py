"""Battery model definition, validation, and OCV interpolation."""

from __future__ import annotations

import bisect
import math
from dataclasses import dataclass, field


class ModelError(ValueError):
    """Raised when battery parameters or the load plan are invalid."""


@dataclass(frozen=True)
class RCBranch:
    resistance_ohm: float
    capacitance_f: float

    @property
    def tau(self) -> float:
        return self.resistance_ohm * self.capacitance_f


@dataclass
class Battery:
    capacity_ah: float
    r0_ohm: float
    ocv_soc: list[float]
    ocv_v: list[float]
    branches: list[RCBranch] = field(default_factory=list)

    @property
    def capacity_c(self) -> float:
        return self.capacity_ah * 3600.0

    def ocv(self, soc: float) -> float:
        """Piecewise-linear OCV; soc must be within the grid range."""
        grid = self.ocv_soc
        if soc <= grid[0]:
            return self.ocv_v[0]
        if soc >= grid[-1]:
            return self.ocv_v[-1]
        i = bisect.bisect_right(grid, soc) - 1
        x0, x1 = grid[i], grid[i + 1]
        y0, y1 = self.ocv_v[i], self.ocv_v[i + 1]
        return y0 + (y1 - y0) * (soc - x0) / (x1 - x0)


def _positive(name: str, value: float) -> float:
    value = float(value)
    if not math.isfinite(value) or value <= 0.0:
        raise ModelError(f"{name} must be positive, got {value}")
    return value


def load_battery(spec: dict) -> Battery:
    try:
        capacity_ah = _positive("capacity_ah", spec["capacity_ah"])
        r0 = float(spec["r0_ohm"])
        if not math.isfinite(r0) or r0 < 0.0:
            raise ModelError(f"r0_ohm must be non-negative, got {r0}")
        grid = spec["ocv_grid"]
        soc_pts = [float(p["soc"]) for p in grid]
        v_pts = [float(p["voltage"]) for p in grid]
    except (KeyError, TypeError) as exc:
        raise ModelError(f"missing or malformed battery field: {exc}") from exc

    if len(soc_pts) < 2:
        raise ModelError("ocv_grid needs at least two points")
    if soc_pts[0] != 0.0 or soc_pts[-1] != 1.0:
        raise ModelError("ocv_grid must cover soc from 0 to 1")
    for a, b in zip(soc_pts, soc_pts[1:]):
        if not b > a:
            raise ModelError("ocv_grid soc values must be strictly increasing")
    for a, b in zip(v_pts, v_pts[1:]):
        if b < a:
            raise ModelError("ocv_grid voltages must be non-decreasing")

    branches = []
    for i, br in enumerate(spec.get("rc_branches", [])):
        r = _positive(f"rc_branches[{i}].r_ohm", br["r_ohm"])
        c = _positive(f"rc_branches[{i}].c_f", br["c_f"])
        branches.append(RCBranch(r, c))

    return Battery(capacity_ah, r0, soc_pts, v_pts, branches)
