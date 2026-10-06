"""Piecewise-linear PV module model and exact string MPP search.

Each module I-V curve is two straight segments:
  (0, Voc) -> (Ik, Vk) -> (Isc, 0)
Beyond Isc the second segment is extrapolated and the module voltage is
clamped at -Vd (bypass diode drop, Vd > 0). A module at or beyond the
clamping current is considered bypassed.
"""

from __future__ import annotations

from dataclasses import dataclass, field


@dataclass(frozen=True)
class Module:
    voc: float
    isc: float
    knee_v: float
    knee_i: float
    diode_drop: float

    def __post_init__(self) -> None:
        if not (0.0 < self.knee_i < self.isc):
            raise ValueError("knee_i must be strictly between 0 and isc")
        if not (0.0 < self.knee_v < self.voc):
            raise ValueError("knee_v must be strictly between 0 and voc")
        if self.diode_drop <= 0.0:
            raise ValueError("diode_drop must be positive")

    @property
    def bypass_current(self) -> float:
        """Current at which the extrapolated second segment reaches -Vd."""
        slope = -self.knee_v / (self.isc - self.knee_i)
        return self.isc + self.diode_drop / -slope

    def voltage(self, current: float) -> float:
        if current <= self.knee_i:
            return self.voc + (self.knee_v - self.voc) * (current / self.knee_i)
        v = self.knee_v * (self.isc - current) / (self.isc - self.knee_i)
        return max(v, -self.diode_drop)

    def is_bypassed(self, current: float) -> bool:
        return current >= self.bypass_current


@dataclass
class ModuleState:
    voltage: float
    bypassed: bool


@dataclass
class OperatingPoint:
    current: float
    voltage: float
    power: float
    modules: list[ModuleState]


@dataclass
class StringAnalysis:
    optimum: OperatingPoint
    local_peaks: list[OperatingPoint]
    max_current: float
    breakpoints: list[float] = field(default_factory=list)


def _string_voltage(modules: list[Module], current: float) -> float:
    return sum(m.voltage(current) for m in modules)


def _make_point(modules: list[Module], current: float) -> OperatingPoint:
    states = [ModuleState(m.voltage(current), m.is_bypassed(current)) for m in modules]
    voltage = sum(s.voltage for s in states)
    return OperatingPoint(current, voltage, current * voltage, states)


def analyze_string(modules: list[Module]) -> StringAnalysis:
    """Find the exact global MPP of a series string.

    Candidate currents are every module knee and bypass-switching point,
    the string zero-voltage current, and the interior extremum of the
    quadratic power on every linear segment in between.
    """
    if not modules:
        raise ValueError("string must contain at least one module")

    points = {0.0}
    for m in modules:
        points.add(m.knee_i)
        points.add(m.bypass_current)
    ordered = sorted(points)

    # Extend the grid until the string voltage goes non-positive so the
    # zero-voltage crossing (upper bound of the generating region) is found.
    i_max = ordered[-1]
    while _string_voltage(modules, i_max) > 0.0:
        i_max *= 2.0
    ordered.append(i_max)

    # Segment endpoints in the generating region (voltage >= 0), inserting
    # the exact zero-voltage crossing current.
    edges: list[float] = [ordered[0]]
    for lo, hi in zip(ordered, ordered[1:]):
        v_lo = _string_voltage(modules, lo)
        v_hi = _string_voltage(modules, hi)
        if v_lo >= 0.0 and v_hi < 0.0:
            # String voltage is linear on this segment.
            crossing = lo + (hi - lo) * v_lo / (v_lo - v_hi)
            if crossing > edges[-1]:
                edges.append(crossing)
            break
        edges.append(hi)
    else:
        raise ValueError("string never reaches zero voltage")

    candidates: list[float] = list(edges)
    for lo, hi in zip(edges, edges[1:]):
        # Power is quadratic on each linear segment: P(I) = I*(a + b*I).
        v_lo = _string_voltage(modules, lo)
        v_hi = _string_voltage(modules, hi)
        slope = (v_hi - v_lo) / (hi - lo)
        intercept = v_lo - slope * lo
        if slope != 0.0:
            extremum = -intercept / (2.0 * slope)
            if lo < extremum < hi:
                candidates.append(extremum)

    evaluated = [
        _make_point(modules, current)
        for current in sorted(set(candidates))
        if _string_voltage(modules, current) >= 0.0
    ]

    best: OperatingPoint | None = None
    for point in evaluated:
        # Ties keep the smaller current (candidates are visited in order).
        if best is None or point.power > best.power:
            best = point

    peaks = [
        point
        for i, point in enumerate(evaluated)
        if point is not best
        and (i == 0 or point.power > evaluated[i - 1].power)
        and (i == len(evaluated) - 1 or point.power > evaluated[i + 1].power)
    ]

    assert best is not None
    return StringAnalysis(best, peaks, edges[-1], edges)
