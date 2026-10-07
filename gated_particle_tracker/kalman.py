"""Constant-velocity Kalman filter for 2D position tracking.

State layout: [x, vx, y, vy]. The measurement is position only:
z = H x + noise with H = [[1, 0, 0, 0], [0, 0, 1, 0]] and covariance
R = measurement_variance * I2.

Process noise follows the continuous white-noise acceleration model:
over an interval dt the two axes are independent and each axis sees a
constant white acceleration with spectral density q (the configured
acceleration variance). The discrete process noise per axis is

    Q1 = q * [[dt^3 / 3, dt^2 / 2],
              [dt^2 / 2, dt      ]]

applied identically to the (x, vx) and (y, vy) blocks.
"""

from __future__ import annotations


def _mat_mul(a, b):
    n = len(a)
    m = len(b[0])
    k = len(b)
    return [
        [sum(a[i][t] * b[t][j] for t in range(k)) for j in range(m)]
        for i in range(n)
    ]


def _mat_add(a, b):
    return [
        [a[i][j] + b[i][j] for j in range(len(a[0]))] for i in range(len(a))
    ]


def _transpose(a):
    return [list(row) for row in zip(*a)]


def _inv2(m):
    det = m[0][0] * m[1][1] - m[0][1] * m[1][0]
    if det <= 0.0:
        raise ArithmeticError("innovation covariance is not positive definite")
    return [
        [m[1][1] / det, -m[0][1] / det],
        [-m[1][0] / det, m[0][0] / det],
    ]


class KalmanState:
    """State estimate (mean + 4x4 covariance) of one track."""

    __slots__ = ("x", "p")

    def __init__(self, x, p):
        self.x = list(x)
        self.p = [list(row) for row in p]

    def copy(self):
        return KalmanState(self.x, self.p)


def new_track_state(obs_x, obs_y, measurement_variance, initial_velocity_variance):
    """New track: velocity zero, position variance = measurement variance."""
    x = [obs_x, 0.0, obs_y, 0.0]
    p = [
        [measurement_variance, 0.0, 0.0, 0.0],
        [0.0, initial_velocity_variance, 0.0, 0.0],
        [0.0, 0.0, measurement_variance, 0.0],
        [0.0, 0.0, 0.0, initial_velocity_variance],
    ]
    return KalmanState(x, p)


def predict(state, dt, acceleration_variance):
    """Predict state forward by dt seconds (dt > 0)."""
    f = [
        [1.0, dt, 0.0, 0.0],
        [0.0, 1.0, 0.0, 0.0],
        [0.0, 0.0, 1.0, dt],
        [0.0, 0.0, 0.0, 1.0],
    ]
    q = acceleration_variance
    dt2 = dt * dt
    dt3 = dt2 * dt
    qm = [
        [q * dt3 / 3.0, q * dt2 / 2.0, 0.0, 0.0],
        [q * dt2 / 2.0, q * dt, 0.0, 0.0],
        [0.0, 0.0, q * dt3 / 3.0, q * dt2 / 2.0],
        [0.0, 0.0, q * dt2 / 2.0, q * dt],
    ]
    x = [sum(f[i][j] * state.x[j] for j in range(4)) for i in range(4)]
    p = _mat_add(_mat_mul(_mat_mul(f, state.p), _transpose(f)), qm)
    return KalmanState(x, p)


def innovation(state, obs_x, obs_y, measurement_variance):
    """Return (residual, innovation covariance) for a position observation."""
    residual = [obs_x - state.x[0], obs_y - state.x[2]]
    s = [
        [state.p[0][0] + measurement_variance, state.p[0][2]],
        [state.p[2][0], state.p[2][2] + measurement_variance],
    ]
    return residual, s


def mahalanobis_squared(residual, s):
    """Squared Mahalanobis distance r^T S^-1 r."""
    s_inv = _inv2(s)
    return (
        residual[0] * (s_inv[0][0] * residual[0] + s_inv[0][1] * residual[1])
        + residual[1] * (s_inv[1][0] * residual[0] + s_inv[1][1] * residual[1])
    )


def update(state, obs_x, obs_y, measurement_variance):
    """Standard Kalman update with a position measurement."""
    residual, s = innovation(state, obs_x, obs_y, measurement_variance)
    s_inv = _inv2(s)
    # P H^T: columns 0 and 2 of P.
    ph = [[state.p[i][0], state.p[i][2]] for i in range(4)]
    k = _mat_mul(ph, s_inv)
    x = [
        state.x[i] + k[i][0] * residual[0] + k[i][1] * residual[1]
        for i in range(4)
    ]
    # Joseph form for numerical symmetry: P = (I-KH)P(I-KH)^T + KRK^T
    ikh = [[(1.0 if i == j else 0.0) for j in range(4)] for i in range(4)]
    for i in range(4):
        ikh[i][0] -= k[i][0]
        ikh[i][2] -= k[i][1]
    r = measurement_variance
    krk = [[k[i][0] * k[j][0] * r + k[i][1] * k[j][1] * r for j in range(4)] for i in range(4)]
    p = _mat_add(_mat_mul(_mat_mul(ikh, state.p), _transpose(ikh)), krk)
    return KalmanState(x, p)
