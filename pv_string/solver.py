"""Exact global MPP search for a series string of piecewise-linear modules.

Total voltage V(I) is the sum of per-module voltages, hence piecewise
linear in I. Power P(I) = I * V(I) is therefore piecewise quadratic.
We collect every module breakpoint (knee, short-circuit, bypass switch),
split the I axis there, and on each segment check both endpoints and the
interior vertex of the quadratic. Only I >= 0 with V(I) >= 0 counts as
generating. Ties are broken toward the smaller current. The result is
independent of any CSV sampling step.
"""

from __future__ import annotations

from dataclasses import dataclass

from .model import Module

TOL = 1e-9


def string_voltage(modules: list[Module], current: float) -> float:
    return sum(m.voltage(current) for m in modules)


def _segment_vertex(modules: list[Module], a: float, b: float) -> float | None:
    """Interior extremum of P(I) on [a, b], or None if degenerate/outside."""
    mid = (a + b) / 2.0
    eps = (b - a) * 1e-6 + 1e-12
    v_mid = string_voltage(modules, mid)
    v_mid2 = string_voltage(modules, mid + eps)
    slope = (v_mid2 - v_mid) / eps
    if abs(slope) < 1e-15:
        return None
    # On the segment V(I) = v_mid + slope * (I - mid); vertex of I*V(I):
    i_star = mid - v_mid / (2.0 * slope)
    if a + TOL < i_star < b - TOL:
        return i_star
    return None


def _zero_crossings(modules: list[Module], points: list[float]) -> list[float]:
    """Currents where V(I) crosses zero inside a segment."""
    crossings: list[float] = []
    for a, b in zip(points, points[1:]):
        va = string_voltage(modules, a)
        vb = string_voltage(modules, b)
        if va > 0.0 > vb or va < 0.0 < vb:
            crossings.append(a - va * (b - a) / (vb - va))
    return crossings


@dataclass(frozen=True)
class Peak:
    current: float
    voltage: float
    power: float
    module_voltages: list[float]
    bypassed: list[bool]


def find_mpp(modules: list[Module]) -> Peak:
    """Global maximum power point over I >= 0, V_string >= 0."""
    points = sorted({0.0} | {p for m in modules for p in m.breakpoints()})
    candidates: set[float] = {0.0}
    candidates.update(points)
    candidates.update(_zero_crossings(modules, points))
    for a, b in zip(points, points[1:]):
        vertex = _segment_vertex(modules, a, b)
        if vertex is not None:
            candidates.add(vertex)

    best: Peak | None = None
    for i in sorted(candidates):
        if i < -TOL:
            continue
        i = max(i, 0.0)
        v = string_voltage(modules, i)
        if v < -TOL:
            continue
        v = max(v, 0.0)
        p = i * v
        if best is None or p > best.power + TOL:
            best = Peak(
                current=i,
                voltage=v,
                power=p,
                module_voltages=[m.voltage(i) for m in modules],
                bypassed=[m.bypassed(i) for m in modules],
            )
    assert best is not None
    return best


def sweep(modules: list[Module], step: float, i_max: float | None = None):
    """Uniform current samples for CSV output (does not affect the MPP)."""
    if step <= 0:
        raise ValueError("step must be positive")
    if i_max is None:
        i_max = max(m.bypass_current for m in modules) * 1.05
    rows = []
    n = int(i_max / step) + 1
    for k in range(n + 1):
        i = k * step
        v = string_voltage(modules, i)
        rows.append((i, v, i * v if v >= 0.0 else 0.0))
    return rows


def analyze_condition(modules: list[Module]) -> dict:
    peak = find_mpp(modules)
    return {
        "current_a": peak.current,
        "voltage_v": peak.voltage,
        "power_w": peak.power,
        "modules": [
            {"index": idx, "voltage_v": mv, "bypassed": bp}
            for idx, (mv, bp) in enumerate(
                zip(peak.module_voltages, peak.bypassed)
            )
        ],
    }
