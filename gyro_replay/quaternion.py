"""Hamilton quaternions, scalar-first (w, x, y, z).

Convention: a quaternion represents the active rotation from the body
frame to the world frame. Composition is q_total = q2 * q1 meaning
"apply q1 first, then q2" (Hamilton product, right-multiplied
increments). All frames are right-handed.
"""

from __future__ import annotations

import math
from dataclasses import dataclass


@dataclass(frozen=True)
class Quaternion:
    w: float
    x: float
    y: float
    z: float

    @staticmethod
    def identity() -> "Quaternion":
        return Quaternion(1.0, 0.0, 0.0, 0.0)

    def norm(self) -> float:
        return math.sqrt(self.w * self.w + self.x * self.x
                         + self.y * self.y + self.z * self.z)

    def normalized(self) -> "Quaternion":
        n = self.norm()
        if n == 0.0:
            raise ValueError("zero quaternion has no attitude")
        return Quaternion(self.w / n, self.x / n, self.y / n, self.z / n)

    def conjugate(self) -> "Quaternion":
        return Quaternion(self.w, -self.x, -self.y, -self.z)

    def __mul__(self, other: "Quaternion") -> "Quaternion":
        """Hamilton product self * other (apply other first, then self)."""
        a, b = self, other
        return Quaternion(
            a.w * b.w - a.x * b.x - a.y * b.y - a.z * b.z,
            a.w * b.x + a.x * b.w + a.y * b.z - a.z * b.y,
            a.w * b.y - a.x * b.z + a.y * b.w + a.z * b.x,
            a.w * b.z + a.x * b.y - a.y * b.x + a.z * b.w,
        )

    def rotate_vector(self, v):
        """Actively rotate body-frame vector v into the world frame."""
        qv = Quaternion(0.0, v[0], v[1], v[2])
        r = self * qv * self.conjugate()
        return (r.x, r.y, r.z)

    @staticmethod
    def from_axis_angle(axis, angle):
        n = math.sqrt(axis[0] ** 2 + axis[1] ** 2 + axis[2] ** 2)
        if n == 0.0:
            return Quaternion.identity()
        half = 0.5 * angle
        s = math.sin(half) / n
        return Quaternion(math.cos(half), axis[0] * s, axis[1] * s, axis[2] * s)

    @staticmethod
    def from_body_rate(omega, dt):
        """Exact increment for constant body-frame rate omega over dt.

        The rotation vector is omega * dt; the increment is right-multiplied
        onto the current attitude by the caller. Zero rate -> identity.
        """
        angle = math.sqrt(omega[0] ** 2 + omega[1] ** 2 + omega[2] ** 2) * dt
        if angle == 0.0:
            return Quaternion.identity()
        return Quaternion.from_axis_angle(omega, angle)

    def attitude_error(self, other: "Quaternion") -> float:
        """Smallest attitude angle to `other`, in [0, pi].

        q and -q denote the same attitude, so we use |dot|.
        """
        a = self.normalized()
        b = other.normalized()
        d = abs(a.w * b.w + a.x * b.x + a.y * b.y + a.z * b.z)
        d = min(1.0, max(-1.0, d))
        return 2.0 * math.acos(d)
