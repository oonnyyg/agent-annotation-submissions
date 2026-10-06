"""Closed-form spin evolution and instantaneous RF pulses.

Conventions: right-handed axes. A positive offset frequency rotates the
transverse magnetization about +z by the right-hand rule. A pulse rotates
the full 3-vector about the transverse axis (cos(phase), sin(phase), 0)
by the flip angle, right-hand rule. All evolution uses exact closed forms
over each interval; there is no time-stepping, so results never depend on
the output sampling grid.
"""

from __future__ import annotations

import cmath
import math


def decay(x):
    """exp(-x) where x = dt/T; T may be infinite (x == 0.0)."""
    return math.exp(-x)


def free_evolve(m, offset_hz, t1, t2, mz_eq, dt):
    """Evolve magnetization m=(Mx,My,Mz) freely for dt seconds."""
    mx, my, mz = m
    if dt <= 0.0:
        return (mx, my, mz)
    theta = 2.0 * math.pi * offset_hz * dt
    c, s = math.cos(theta), math.sin(theta)
    e2 = 1.0 if math.isinf(t2) else math.exp(-dt / t2)
    e1 = 1.0 if math.isinf(t1) else math.exp(-dt / t1)
    rx = (mx * c - my * s) * e2
    ry = (mx * s + my * c) * e2
    rz = mz if math.isinf(t1) else mz_eq + (mz - mz_eq) * e1
    return (rx, ry, rz)


def apply_pulse(m, phase, flip):
    """Rotate m about transverse axis (cos(phase), sin(phase), 0) by flip."""
    mx, my, mz = m
    ux, uy = math.cos(phase), math.sin(phase)
    c, s = math.cos(flip), math.sin(flip)
    dot = ux * mx + uy * my
    rx = mx * c + uy * mz * s + ux * dot * (1.0 - c)
    ry = my * c - ux * mz * s + uy * dot * (1.0 - c)
    rz = mz * c + (ux * my - uy * mx) * s
    return (rx, ry, rz)
