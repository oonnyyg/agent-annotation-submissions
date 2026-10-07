"""2D constant-velocity Kalman filter with position-only measurements.

State per axis: [position, velocity]. The full state is
[x, vx, y, vy] and the two axes are filtered independently with
identical parameters, so we implement a single 2x2 filter and apply
it to each axis.

Process model per axis over interval dt:
    F = [[1, dt], [0, 1]]
    Q = q * [[dt^3/3, dt^2/2], [dt^2/2, dt]]
where q is the acceleration variance (continuous white-noise
acceleration model). Measurement: z = position + noise, variance r.
"""

from __future__ import annotations

import math
from dataclasses import dataclass, field


def _check_finite(value: float, name: str) -> float:
    value = float(value)
    if not math.isfinite(value):
        raise ValueError(f"{name} must be finite, got {value!r}")
    return value


@dataclass
class AxisFilter:
    """Scalar [position, velocity] Kalman filter."""

    pos: float
    vel: float
    p00: float
    p01: float
    p11: float

    def predict(self, dt: float, accel_var: float) -> None:
        p00, p01, p11 = self.p00, self.p01, self.p11
        self.pos = self.pos + dt * self.vel
        self.p00 = p00 + dt * (p01 + p01) + dt * dt * p11 + accel_var * dt**3 / 3.0
        self.p01 = p01 + dt * p11 + accel_var * dt * dt / 2.0
        self.p11 = p11 + accel_var * dt

    def innovation(self, meas: float, meas_var: float) -> tuple[float, float]:
        """Return (residual, innovation variance)."""
        return meas - self.pos, self.p00 + meas_var

    def update(self, meas: float, meas_var: float) -> None:
        residual, s = self.innovation(meas, meas_var)
        k0 = self.p00 / s
        k1 = self.p01 / s
        p00, p01, p11 = self.p00, self.p01, self.p11
        self.pos += k0 * residual
        self.vel += k1 * residual
        self.p00 = (1.0 - k0) * p00
        self.p01 = (1.0 - k0) * p01
        self.p11 = p11 - k1 * p01


@dataclass
class KalmanState:
    """Full 2D state: two independent axis filters."""

    x_axis: AxisFilter
    y_axis: AxisFilter

    @classmethod
    def from_measurement(
        cls, x: float, y: float, meas_var: float, init_vel_var: float
    ) -> "KalmanState":
        return cls(
            AxisFilter(x, 0.0, meas_var, 0.0, init_vel_var),
            AxisFilter(y, 0.0, meas_var, 0.0, init_vel_var),
        )

    @property
    def position(self) -> tuple[float, float]:
        return self.x_axis.pos, self.y_axis.pos

    def predict(self, dt: float, accel_var: float) -> None:
        self.x_axis.predict(dt, accel_var)
        self.y_axis.predict(dt, accel_var)

    def innovation(self, x: float, y: float, meas_var: float) -> tuple[tuple[float, float], float, float]:
        """Return ((rx, ry), sx, sy): residuals and per-axis innovation variances."""
        rx, sx = self.x_axis.innovation(x, meas_var)
        ry, sy = self.y_axis.innovation(y, meas_var)
        return (rx, ry), sx, sy

    def mahalanobis_sq(self, x: float, y: float, meas_var: float) -> float:
        (rx, ry), sx, sy = self.innovation(x, y, meas_var)
        return rx * rx / sx + ry * ry / sy

    def update(self, x: float, y: float, meas_var: float) -> None:
        self.x_axis.update(x, meas_var)
        self.y_axis.update(y, meas_var)

    def position_covariance(self) -> list[list[float]]:
        return [
            [self.x_axis.p00, 0.0],
            [0.0, self.y_axis.p00],
        ]
