"""Integrate body-frame gyro rates into attitude over time."""

from __future__ import annotations

import bisect
from .quaternion import Quaternion


class GyroReplay:
    """Piecewise-constant body-rate integrator.

    times: strictly increasing sample times (seconds).
    rates: body rates (wx, wy, wz) in rad/s; rates[i] holds over
        [times[i], times[i+1]). bias is subtracted per axis first.
    q0: initial body-to-world attitude (normalized; zero rejected).
    """

    def __init__(self, times, rates, bias=(0.0, 0.0, 0.0), q0=None):
        if len(times) < 2:
            raise ValueError("need at least two sample times")
        if len(rates) != len(times) - 1:
            raise ValueError("rates must have len(times) - 1 entries")
        for a, b in zip(times, times[1:]):
            if not b > a:
                raise ValueError("times must be strictly increasing")
        self.times = list(times)
        self.rates = [tuple(r[j] - bias[j] for j in range(3)) for r in rates]
        self.q0 = (q0 if q0 is not None else Quaternion.identity()).normalized()
        # Attitude at each knot: q[i+1] = q[i] * dq_i (right-multiply).
        self._knots = [self.q0]
        for i, rate in enumerate(self.rates):
            dt = self.times[i + 1] - self.times[i]
            dq = Quaternion.from_body_rate(rate, dt)
            self._knots.append((self._knots[-1] * dq).normalized())

    @property
    def t_start(self):
        return self.times[0]

    @property
    def t_end(self):
        return self.times[-1]

    def attitude(self, t):
        """Attitude at any t in [t_start, t_end]; no extrapolation.

        Integrates the segment's exact constant-rate increment up to t,
        so rotations longer than half a turn evolve along the actual
        rate vector instead of taking a shortest arc between endpoints.
        """
        if not (self.times[0] <= t <= self.times[-1]):
            raise ValueError(f"t={t} outside [{self.times[0]}, {self.times[-1]}]")
        i = bisect.bisect_right(self.times, t) - 1
        i = min(i, len(self.rates) - 1)
        dt = t - self.times[i]
        if dt == 0.0:
            return self._knots[i]
        dq = Quaternion.from_body_rate(self.rates[i], dt)
        return (self._knots[i] * dq).normalized()

    def rotate_to_world(self, t, vector):
        """Rotate a body-frame vector into the world frame at time t."""
        return self.attitude(t).rotate_vector(vector)
