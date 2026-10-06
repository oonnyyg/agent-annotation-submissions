"""Per-module piecewise-linear V(I) model.

Each module is defined by:
  - voc: open-circuit voltage (V > 0)
  - isc: short-circuit current (A > 0)
  - vknee, iknee: knee point, strictly inside (0, voc) x (0, isc)
  - diode_drop: bypass diode forward drop (V > 0)

V(I) is linear from (0, voc) to (iknee, vknee), then linear from
(iknee, vknee) to (isc, 0). Beyond isc the second segment is
extrapolated and the voltage is clamped at -diode_drop (module
bypassed by its diode).
"""

from __future__ import annotations

from dataclasses import dataclass


@dataclass(frozen=True)
class Module:
    voc: float
    isc: float
    vknee: float
    iknee: float
    diode_drop: float = 0.5

    def __post_init__(self) -> None:
        if self.voc <= 0:
            raise ValueError("voc must be positive")
        if self.isc <= 0:
            raise ValueError("isc must be positive")
        if not 0.0 < self.vknee < self.voc:
            raise ValueError("vknee must lie strictly between 0 and voc")
        if not 0.0 < self.iknee < self.isc:
            raise ValueError("iknee must lie strictly between 0 and isc")
        if self.diode_drop <= 0:
            raise ValueError("diode_drop must be positive")

    @property
    def slope1(self) -> float:
        """dV/dI on the open-circuit-to-knee segment (negative)."""
        return (self.vknee - self.voc) / self.iknee

    @property
    def slope2(self) -> float:
        """dV/dI on the knee-to-short-circuit segment (negative)."""
        return (0.0 - self.vknee) / (self.isc - self.iknee)

    @property
    def bypass_current(self) -> float:
        """Current at which the extrapolated segment reaches -diode_drop."""
        return self.isc + self.diode_drop / (-self.slope2)

    def breakpoints(self) -> list[float]:
        """Currents where this module's V(I) changes slope."""
        return [self.iknee, self.isc, self.bypass_current]

    def voltage(self, current: float) -> float:
        """Module voltage at the given current (clamped at -diode_drop)."""
        i = current
        if i <= self.iknee:
            v = self.voc + self.slope1 * i
        else:
            v = self.vknee + self.slope2 * (i - self.iknee)
        return max(v, -self.diode_drop)

    def bypassed(self, current: float, tol: float = 1e-9) -> bool:
        """True when the module sits at (or below) the diode clamp."""
        return self.voltage(current) <= -self.diode_drop + tol
