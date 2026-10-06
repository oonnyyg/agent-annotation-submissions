"""Core quaternion math and gyroscope attitude integration."""

from __future__ import annotations

import bisect
import csv
import json
import math
from dataclasses import dataclass
from pathlib import Path
from typing import Iterator, Sequence


def _require_finite(values: Sequence[float], what: str) -> None:
    if not all(math.isfinite(v) for v in values):
        raise ValueError(f"{what} must contain only finite numbers")


@dataclass(frozen=True)
class Quaternion:
    """Hamilton quaternion, scalar first, body-to-world active rotation."""

    w: float
    x: float
    y: float
    z: float

    @classmethod
    def identity(cls) -> "Quaternion":
        return cls(1.0, 0.0, 0.0, 0.0)

    @classmethod
    def from_sequence(cls, values: Sequence[float]) -> "Quaternion":
        if len(values) != 4:
            raise ValueError("quaternion needs exactly 4 components (w, x, y, z)")
        _require_finite(values, "quaternion")
        q = cls(*map(float, values))
        norm = q.norm()
        if norm == 0.0:
            raise ValueError("zero quaternion does not represent a rotation")
        return q / norm

    def norm(self) -> float:
        return math.sqrt(self.w**2 + self.x**2 + self.y**2 + self.z**2)

    def normalized(self) -> "Quaternion":
        n = self.norm()
        if n == 0.0:
            raise ValueError("cannot normalize a zero quaternion")
        return self / n

    def __truediv__(self, scalar: float) -> "Quaternion":
        return Quaternion(self.w / scalar, self.x / scalar,
                          self.y / scalar, self.z / scalar)

    def __mul__(self, other: "Quaternion") -> "Quaternion":
        a, b = self, other
        return Quaternion(
            a.w * b.w - a.x * b.x - a.y * b.y - a.z * b.z,
            a.w * b.x + a.x * b.w + a.y * b.z - a.z * b.y,
            a.w * b.y - a.x * b.z + a.y * b.w + a.z * b.x,
            a.w * b.z + a.x * b.y - a.y * b.x + a.z * b.w,
        )

    def conjugate(self) -> "Quaternion":
        return Quaternion(self.w, -self.x, -self.y, -self.z)

    def dot(self, other: "Quaternion") -> float:
        return (self.w * other.w + self.x * other.x
                + self.y * other.y + self.z * other.z)

    def rotate(self, vector: Sequence[float]) -> tuple[float, float, float]:
        """Actively rotate a body-frame vector into the world frame."""
        if len(vector) != 3:
            raise ValueError("vector needs exactly 3 components")
        _require_finite(vector, "vector")
        v = Quaternion(0.0, *map(float, vector))
        r = self * v * self.conjugate()
        return (r.x, r.y, r.z)

    @classmethod
    def from_axis_angle(cls, axis: Sequence[float], angle: float) -> "Quaternion":
        _require_finite(axis, "axis")
        if not math.isfinite(angle):
            raise ValueError("angle must be finite")
        n = math.sqrt(sum(c * c for c in axis))
        if n == 0.0:
            return cls.identity()
        half = 0.5 * angle
        s = math.sin(half) / n
        return cls(math.cos(half), axis[0] * s, axis[1] * s, axis[2] * s)

    @classmethod
    def from_body_rate(cls, omega: Sequence[float], dt: float) -> "Quaternion":
        """Exact increment of constant body-frame rate ``omega`` over ``dt``."""
        if len(omega) != 3:
            raise ValueError("omega needs exactly 3 components")
        _require_finite(omega, "omega")
        if not math.isfinite(dt):
            raise ValueError("dt must be finite")
        angle = math.sqrt(sum(w * w for w in omega)) * dt
        if angle == 0.0:
            return cls.identity()
        return cls.from_axis_angle(omega, angle)

    def angle_to(self, other: "Quaternion") -> float:
        """Smallest attitude angle to ``other`` in [0, pi]; sign-invariant."""
        d = abs(self.normalized().dot(other.normalized()))
        return 2.0 * math.acos(min(1.0, d))


def load_gyro_csv(path: str | Path) -> list[tuple[float, tuple[float, float, float]]]:
    """Load ``time,wx,wy,wz`` rows; times strictly increasing, values finite.

    Each row's rate applies from its own time up to the next row's time;
    the last row only fixes the end of the record.
    """
    rows: list[tuple[float, tuple[float, float, float]]] = []
    with open(path, newline="", encoding="utf-8") as fh:
        for lineno, record in enumerate(csv.reader(fh), start=1):
            if not record or all(not c.strip() for c in record):
                continue
            if record[0].strip().startswith("#"):
                continue
            if len(record) != 4:
                raise ValueError(f"{path}:{lineno}: expected 4 columns")
            try:
                t, wx, wy, wz = (float(c) for c in record)
            except ValueError as exc:
                raise ValueError(f"{path}:{lineno}: non-numeric field") from exc
            _require_finite((t, wx, wy, wz), f"{path}:{lineno}")
            if rows and t <= rows[-1][0]:
                raise ValueError(f"{path}:{lineno}: times must strictly increase")
            rows.append((t, (wx, wy, wz)))
    if len(rows) < 2:
        raise ValueError(f"{path}: need at least two gyro rows")
    return rows


def load_bias_json(path: str | Path) -> tuple[float, float, float]:
    """Load a fixed body-frame bias ``{"bias": [bx, by, bz]}`` in rad/s."""
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    try:
        bias = data["bias"]
    except (TypeError, KeyError) as exc:
        raise ValueError(f"{path}: expected an object with a 'bias' key") from exc
    if len(bias) != 3:
        raise ValueError(f"{path}: bias needs exactly 3 components")
    bias = tuple(float(b) for b in bias)
    _require_finite(bias, f"{path}: bias")
    return bias  # type: ignore[return-value]


def load_reference_csv(path: str | Path) -> list[tuple[float, Quaternion]]:
    """Load ``time,qw,qx,qy,qz`` reference attitudes."""
    out: list[tuple[float, Quaternion]] = []
    with open(path, newline="", encoding="utf-8") as fh:
        for lineno, record in enumerate(csv.reader(fh), start=1):
            if not record or all(not c.strip() for c in record):
                continue
            if record[0].strip().startswith("#"):
                continue
            if len(record) != 5:
                raise ValueError(f"{path}:{lineno}: expected 5 columns")
            try:
                values = [float(c) for c in record]
            except ValueError as exc:
                raise ValueError(f"{path}:{lineno}: non-numeric field") from exc
            try:
                q = Quaternion.from_sequence(values[1:])
            except ValueError as exc:
                raise ValueError(f"{path}:{lineno}: {exc}") from exc
            out.append((values[0], q))
    return out


class AttitudeReplay:
    """Integrate bias-corrected body rates into an attitude timeline.

    Integration is exact per segment: each segment keeps its row's constant
    body-frame rate, the increment is right-multiplied, and the result is
    renormalized.  Queries integrate from the segment start to the query
    time along the segment's actual rate, so rotations beyond half a turn
    inside one segment are followed faithfully (no shortest-arc slerp).
    """

    def __init__(
        self,
        rows: Sequence[tuple[float, Sequence[float]]],
        initial: Quaternion,
        bias: Sequence[float] = (0.0, 0.0, 0.0),
    ) -> None:
        if len(rows) < 2:
            raise ValueError("need at least two gyro rows")
        if len(bias) != 3:
            raise ValueError("bias needs exactly 3 components")
        _require_finite(bias, "bias")
        self._bias = tuple(float(b) for b in bias)
        self._times: list[float] = []
        self._rates: list[tuple[float, float, float]] = []
        self._attitudes: list[Quaternion] = []
        q = initial.normalized()
        prev_t: float | None = None
        for t, omega in rows:
            _require_finite((t, *omega), "gyro row")
            if prev_t is not None and t <= prev_t:
                raise ValueError("gyro times must strictly increase")
            corrected = tuple(w - b for w, b in zip(omega, self._bias))
            if prev_t is not None:
                dq = Quaternion.from_body_rate(self._rates[-1], t - prev_t)
                q = (q * dq).normalized()
            self._times.append(float(t))
            self._rates.append(corrected)  # type: ignore[arg-type]
            self._attitudes.append(q)
            prev_t = t

    @property
    def start(self) -> float:
        return self._times[0]

    @property
    def end(self) -> float:
        return self._times[-1]

    def attitude(self, t: float) -> Quaternion:
        """Attitude at time ``t``; must lie inside the record (no extrapolation)."""
        if not math.isfinite(t):
            raise ValueError("query time must be finite")
        if t < self._times[0] or t > self._times[-1]:
            raise ValueError(
                f"query time {t} outside record [{self.start}, {self.end}]"
            )
        i = bisect.bisect_right(self._times, t) - 1
        i = min(i, len(self._times) - 2)
        dt = t - self._times[i]
        if dt == 0.0:
            return self._attitudes[i]
        dq = Quaternion.from_body_rate(self._rates[i], dt)
        return (self._attitudes[i] * dq).normalized()

    def rotate_to_world(self, t: float, vector: Sequence[float]) -> tuple[float, float, float]:
        """Rotate a body-frame vector into the world frame at time ``t``."""
        return self.attitude(t).rotate(vector)

    def error_against(self, t: float, reference: Quaternion) -> float:
        """Smallest attitude angle to a reference quaternion, in [0, pi]."""
        return self.attitude(t).angle_to(reference)

    def iter_attitudes(self) -> Iterator[tuple[float, Quaternion]]:
        """Yield ``(time, attitude)`` at every gyro row time."""
        return iter(zip(self._times, self._attitudes))
