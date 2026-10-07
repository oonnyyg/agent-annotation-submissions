"""Filtered backprojection core (standard library only).

Fourier convention: frequencies are measured in cycles per unit length.
For a projection sampled with spacing ``s`` and zero-padded to length
``L``, bin ``k`` corresponds to frequency ``k / (L * s)`` (with the usual
wrap-around for the negative half).  The ramp multiplier is ``abs(f)``.
Because ``sum_n p_n exp(-2 pi i f t_n) * s`` approximates the continuous
Fourier transform and the frequency sample spacing is ``1 / (L s)``, the
inverse DFT of ``P_k * |f_k|`` (with the 1/L inverse normalisation)
directly approximates the continuous filtered projection
``integral |f| P(f) exp(2 pi i f t) df`` -- no extra scale factors.
"""

from __future__ import annotations

import cmath
import math

from .config import Geometry


def _dft(signal: list[complex], inverse: bool) -> list[complex]:
    n = len(signal)
    sign = 1.0 if inverse else -1.0
    out = []
    for k in range(n):
        acc = 0j
        for j, x in enumerate(signal):
            acc += x * cmath.exp(sign * 2j * math.pi * j * k / n)
        if inverse:
            acc /= n
        out.append(acc)
    return out


def _next_pow2(n: int) -> int:
    p = 1
    while p < n:
        p <<= 1
    return p


def ramp_filter(readings: list[float], spacing: float) -> list[float]:
    """Apply the ramp filter |f| (cycles per unit length) to one projection.

    The signal is zero-padded to at least twice its length (next power of
    two), transformed, multiplied by |f|, inverse transformed and cropped
    back to the original detector positions.  No normalisation of the
    maximum or any other ad-hoc rescaling is applied.
    """
    n = len(readings)
    padded = _next_pow2(2 * n)
    signal = [complex(v, 0.0) for v in readings] + [0j] * (padded - n)
    spectrum = _dft(signal, inverse=False)
    filtered = []
    for k, coeff in enumerate(spectrum):
        kk = k if k <= padded // 2 else k - padded
        freq = kk / (padded * spacing)
        filtered.append(coeff * abs(freq))
    time_domain = _dft(filtered, inverse=True)
    return [time_domain[i].real for i in range(n)]


def _interpolate(positions: list[float], values: list[float], t: float) -> float:
    if t < positions[0] or t > positions[-1]:
        return 0.0
    spacing = positions[1] - positions[0]
    u = (t - positions[0]) / spacing
    i = int(math.floor(u))
    if i >= len(values) - 1:
        return values[-1]
    frac = u - i
    return values[i] * (1.0 - frac) + values[i + 1] * frac


def reconstruct(
    projections: list[tuple[float, list[float]]], geometry: Geometry
) -> list[list[float]]:
    """Reconstruct a 2D matrix from filtered projections.

    ``projections`` is a list of ``(angle_radians, readings)``.  Each
    projection is ramp filtered, then each pixel centre is projected onto
    the detector coordinate ``t = x cos(theta) + y sin(theta)`` and the
    filtered signal is linearly interpolated (zero outside the detector
    range).  Contributions are summed and multiplied by the angular step
    in radians.  Negative values are preserved.
    """
    positions = geometry.detector_positions()
    filtered = [
        ramp_filter(readings, geometry.detector_spacing)
        for _, readings in projections
    ]
    dtheta = math.pi / len(projections)

    image = [[0.0] * geometry.nx for _ in range(geometry.ny)]
    cx = (geometry.nx - 1) / 2.0
    cy = (geometry.ny - 1) / 2.0
    for iy in range(geometry.ny):
        y = (iy - cy) * geometry.dy
        for ix in range(geometry.nx):
            x = (ix - cx) * geometry.dx
            total = 0.0
            for (angle, _), fproj in zip(projections, filtered):
                t = x * math.cos(angle) + y * math.sin(angle)
                total += _interpolate(positions, fproj, t)
            image[iy][ix] = total * dtheta
    return image
