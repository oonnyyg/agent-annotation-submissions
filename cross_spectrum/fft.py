"""Small pure-Python FFT (radix-2) with a direct-DFT fallback."""

from __future__ import annotations

import cmath
import math


def fft(values: list[complex]) -> list[complex]:
    n = len(values)
    if n == 0:
        return []
    if n & (n - 1):
        return _dft(values)
    return _fft_radix2(values)


def _fft_radix2(values: list[complex]) -> list[complex]:
    n = len(values)
    if n == 1:
        return list(values)
    even = _fft_radix2(values[0::2])
    odd = _fft_radix2(values[1::2])
    out = [0j] * n
    half = n // 2
    for k in range(half):
        twiddle = cmath.exp(-2j * math.pi * k / n) * odd[k]
        out[k] = even[k] + twiddle
        out[k + half] = even[k] - twiddle
    return out


def _dft(values: list[complex]) -> list[complex]:
    n = len(values)
    out = []
    for k in range(n):
        acc = 0j
        for t, v in enumerate(values):
            acc += v * cmath.exp(-2j * math.pi * k * t / n)
        out.append(acc)
    return out
