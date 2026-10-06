"""Welch-averaged auto/cross spectra, coherence and phase."""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .config import Config
from .fft import fft


class AnalysisError(RuntimeError):
    """Raised when no valid segment can be analysed."""


@dataclass
class SegmentSkip:
    start_index: int
    reason: str


@dataclass
class AnalysisResult:
    frequencies: list[float]
    psd1: list[float]
    psd2: list[float]
    cross: list[complex]
    coherence: list[float | None]
    phase: list[float | None]
    valid_segments: int
    skipped: list[SegmentSkip] = field(default_factory=list)
    energy_check: dict = field(default_factory=dict)


def make_window(kind: str, length: int) -> list[float]:
    if kind == "rectangular":
        return [1.0] * length
    return [0.5 - 0.5 * math.cos(2.0 * math.pi * n / length) for n in range(length)]


def _extract_segment(samples, start, length):
    """Return (ch1, ch2) lists, or a skip reason string."""
    ch1: list[float] = []
    ch2: list[float] = []
    missing_rows = 0
    missing_ch1 = 0
    missing_ch2 = 0
    for offset in range(length):
        record = samples.get(start + offset)
        if record is None:
            missing_rows += 1
            continue
        v1, v2 = record
        if v1 is None:
            missing_ch1 += 1
        if v2 is None:
            missing_ch2 += 1
        ch1.append(v1 if v1 is not None else 0.0)
        ch2.append(v2 if v2 is not None else 0.0)
    if missing_rows:
        return f"{missing_rows} sample index(es) absent from CSV"
    if missing_ch1 or missing_ch2:
        parts = []
        if missing_ch1:
            parts.append(f"ch1 missing at {missing_ch1} sample(s)")
        if missing_ch2:
            parts.append(f"ch2 missing at {missing_ch2} sample(s)")
        return "; ".join(parts)
    return ch1, ch2


def analyze(samples: dict, config: Config) -> AnalysisResult:
    seg = config.segment_length
    nfft = config.nfft
    window = make_window(config.window, seg)
    window_power = sum(w * w for w in window)
    norm = config.sample_rate * window_power

    first_index = min(samples)
    last_index = max(samples)
    half = nfft // 2 + 1

    acc_psd1 = [0.0] * half
    acc_psd2 = [0.0] * half
    acc_cross = [0j] * half
    energy1 = 0.0
    energy2 = 0.0
    valid = 0
    skipped: list[SegmentSkip] = []

    start = first_index
    while start + seg - 1 <= last_index:
        extracted = _extract_segment(samples, start, seg)
        if isinstance(extracted, str):
            skipped.append(SegmentSkip(start, extracted))
            start += config.step
            continue
        ch1, ch2 = extracted
        mean1 = sum(ch1) / seg
        mean2 = sum(ch2) / seg
        buf1 = [(v - mean1) * w for v, w in zip(ch1, window)]
        buf2 = [(v - mean2) * w for v, w in zip(ch2, window)]
        energy1 += sum(v * v for v in buf1)
        energy2 += sum(v * v for v in buf2)
        buf1.extend([0.0] * (nfft - seg))
        buf2.extend([0.0] * (nfft - seg))
        spec1 = fft([complex(v, 0.0) for v in buf1])
        spec2 = fft([complex(v, 0.0) for v in buf2])
        for k in range(half):
            acc_psd1[k] += (spec1[k].real ** 2 + spec1[k].imag ** 2) / norm
            acc_psd2[k] += (spec2[k].real ** 2 + spec2[k].imag ** 2) / norm
            acc_cross[k] += spec1[k].conjugate() * spec2[k] / norm
        valid += 1
        start += config.step

    if valid == 0:
        raise AnalysisError(
            f"no valid segment: all {len(skipped)} candidate segment(s) were skipped"
        )

    psd1 = [v / valid for v in acc_psd1]
    psd2 = [v / valid for v in acc_psd2]
    cross = [v / valid for v in acc_cross]

    # One-sided doubling, except DC and the Nyquist bin (present for even nfft).
    last_bin = half - 1
    for k in range(1, half):
        if k == last_bin and nfft % 2 == 0:
            continue
        psd1[k] *= 2.0
        psd2[k] *= 2.0
        cross[k] *= 2.0

    coherence: list[float | None] = []
    phase: list[float | None] = []
    for k in range(half):
        product = psd1[k] * psd2[k]
        if product == 0.0:
            coherence.append(None)
        else:
            value = abs(cross[k]) ** 2 / product
            coherence.append(min(1.0, value))
        if cross[k] == 0j:
            phase.append(None)
        else:
            phase.append(math.atan2(cross[k].imag, cross[k].real))

    df = config.sample_rate / nfft
    frequencies = [k * df for k in range(half)]
    energy_check = {
        "ch1": {
            "psd_integral": sum(psd1) * df,
            "mean_windowed_energy": energy1 / valid / window_power,
        },
        "ch2": {
            "psd_integral": sum(psd2) * df,
            "mean_windowed_energy": energy2 / valid / window_power,
        },
    }
    for channel in energy_check.values():
        reference = channel["mean_windowed_energy"]
        channel["relative_error"] = (
            abs(channel["psd_integral"] - reference) / reference if reference else None
        )

    return AnalysisResult(
        frequencies=frequencies,
        psd1=psd1,
        psd2=psd2,
        cross=cross,
        coherence=coherence,
        phase=phase,
        valid_segments=valid,
        skipped=skipped,
        energy_check=energy_check,
    )
