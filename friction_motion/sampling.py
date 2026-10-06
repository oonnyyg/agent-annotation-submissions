"""Sampling of the analytic trajectory at uniform time steps."""

from __future__ import annotations

from .core import _Piece


def sample_rows(pieces: list[_Piece], t_end: float, dt: float):
    """Yield (t, x, v) rows at t = 0, dt, 2*dt, ..., t_end.

    Sampling never affects the dynamics or energy totals; values are
    evaluated analytically from the piecewise solution."""
    eps = 1e-12 * max(1.0, t_end)
    n = 0
    while True:
        t = n * dt
        if t > t_end + eps:
            break
        t = min(t, t_end)
        x, v = _eval_at(pieces, t)
        yield t, x, v
        n += 1
    if (n - 1) * dt < t_end - eps:
        x, v = _eval_at(pieces, t_end)
        yield t_end, x, v


def _eval_at(pieces: list[_Piece], t: float) -> tuple[float, float]:
    for piece in pieces:
        if piece.t0 <= t <= piece.t1:
            return piece.state_at(t)
    last = pieces[-1]
    return last.state_at(last.t1)
