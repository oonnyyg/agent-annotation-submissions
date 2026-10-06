"""Core Welch cross-spectrum computation (standard library only)."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass, field
from pathlib import Path

MAX_SEGMENT_LENGTH = 512
MIN_SEGMENT_LENGTH = 3


class ConfigError(ValueError):
    """Raised when the JSON configuration is invalid."""


class DataError(ValueError):
    """Raised when the input CSV cannot be used."""


@dataclass(frozen=True)
class Config:
    sample_rate: float
    segment_length: int
    overlap: int
    nfft: int
    window: str  # "rect" or "hann"

    @property
    def step(self) -> int:
        return self.segment_length - self.overlap


def load_config(path: str | Path) -> Config:
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    if not isinstance(raw, dict):
        raise ConfigError("config root must be a JSON object")
    try:
        sample_rate = float(raw["sample_rate"])
        segment_length = int(raw["segment_length"])
        overlap = int(raw["overlap"])
        nfft = int(raw["nfft"])
        window = str(raw.get("window", "hann")).lower()
    except KeyError as exc:
        raise ConfigError(f"missing config key: {exc.args[0]}") from exc
    except (TypeError, ValueError) as exc:
        raise ConfigError(f"invalid config value: {exc}") from exc

    if not math.isfinite(sample_rate) or sample_rate <= 0:
        raise ConfigError("sample_rate must be a positive finite number")
    if not (MIN_SEGMENT_LENGTH <= segment_length <= MAX_SEGMENT_LENGTH):
        raise ConfigError(
            f"segment_length must be between {MIN_SEGMENT_LENGTH} and "
            f"{MAX_SEGMENT_LENGTH}"
        )
    if overlap < 0 or overlap >= segment_length:
        raise ConfigError("overlap must be non-negative and < segment_length")
    if nfft < segment_length:
        raise ConfigError("nfft must not be shorter than segment_length")
    if window not in ("rect", "hann"):
        raise ConfigError("window must be 'rect' or 'hann'")
    return Config(sample_rate, segment_length, overlap, nfft, window)


def read_csv_channels(path: str | Path) -> tuple[list[float | None], list[float | None]]:
    """Read a two-channel CSV; missing/empty cells become None.

    Rows are located by zero-based sample index; every row must have the
    same sample spacing (one row per sample). A header row is detected
    and skipped.
    """
    ch1: list[float | None] = []
    ch2: list[float | None] = []
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.reader(fh)
        for lineno, row in enumerate(reader, start=1):
            if not row or all(not cell.strip() for cell in row):
                continue
            if lineno == 1 and not _is_number(row[0]):
                continue  # header row
            if len(row) < 2:
                raise DataError(f"line {lineno}: expected 2 columns, got {len(row)}")
            ch1.append(_parse_cell(row[0], lineno))
            ch2.append(_parse_cell(row[1], lineno))
    if not ch1:
        raise DataError("CSV contains no data rows")
    return ch1, ch2


def _is_number(text: str) -> bool:
    try:
        float(text.strip())
        return True
    except ValueError:
        return False


def _parse_cell(text: str, lineno: int) -> float | None:
    stripped = text.strip()
    if stripped == "" or stripped.lower() in ("nan", "na", "null"):
        return None
    try:
        value = float(stripped)
    except ValueError as exc:
        raise DataError(f"line {lineno}: non-numeric value {stripped!r}") from exc
    if not math.isfinite(value):
        return None
    return value


@dataclass
class SegmentStats:
    total_candidates: int = 0
    used: int = 0
    skipped_missing: int = 0
    skipped_missing_starts: list[int] = field(default_factory=list)


@dataclass
class SpectrumResult:
    frequencies: list[float]
    psd1: list[float]
    psd2: list[float]
    cross: list[complex]
    coherence: list[float | None]
    phase: list[float | None]
    stats: SegmentStats
    window_power: float  # sum of w^2 over the segment window
    energy_check: dict


def make_window(kind: str, length: int) -> list[float]:
    if kind == "rect":
        return [1.0] * length
    # periodic Hann
    return [0.5 - 0.5 * math.cos(2.0 * math.pi * n / length) for n in range(length)]


def dft(samples: list[float], nfft: int) -> list[complex]:
    """Direct DFT of a zero-padded real sequence (nfft <= 512+)."""
    out = []
    for k in range(nfft):
        acc = 0j
        for n, x in enumerate(samples):
            angle = -2.0 * math.pi * k * n / nfft
            acc += x * complex(math.cos(angle), math.sin(angle))
        out.append(acc)
    return out


def compute_cross_spectrum(
    ch1: list[float | None],
    ch2: list[float | None],
    config: Config,
) -> SpectrumResult:
    if len(ch1) != len(ch2):
        raise DataError("channels have different lengths")
    n = len(ch1)
    seg = config.segment_length
    step = config.step
    nfft = config.nfft
    window = make_window(config.window, seg)
    window_power = sum(w * w for w in window)
    norm = config.sample_rate * window_power

    stats = SegmentStats()
    sum_p1 = [0.0] * nfft
    sum_p2 = [0.0] * nfft
    sum_cross = [0j] * nfft
    # accumulators for the energy consistency check
    energy_time = 0.0  # sum of windowed segment energy (both channels)

    start = 0
    while start + seg <= n:
        stats.total_candidates += 1
        s1 = ch1[start : start + seg]
        s2 = ch2[start : start + seg]
        if any(v is None for v in s1) or any(v is None for v in s2):
            stats.skipped_missing += 1
            stats.skipped_missing_starts.append(start)
            start += step
            continue
        x1 = [float(v) for v in s1]
        x2 = [float(v) for v in s2]
        m1 = sum(x1) / seg
        m2 = sum(x2) / seg
        w1 = [(x1[i] - m1) * window[i] for i in range(seg)] + [0.0] * (nfft - seg)
        w2 = [(x2[i] - m2) * window[i] for i in range(seg)] + [0.0] * (nfft - seg)
        energy_time += sum(v * v for v in w1) + sum(v * v for v in w2)
        f1 = dft(w1, nfft)
        f2 = dft(w2, nfft)
        for k in range(nfft):
            sum_p1[k] += (f1[k].real ** 2 + f1[k].imag ** 2) / norm
            sum_p2[k] += (f2[k].real ** 2 + f2[k].imag ** 2) / norm
            sum_cross[k] += f1[k].conjugate() * f2[k] / norm
        stats.used += 1
        start += step

    if stats.used == 0:
        raise DataError(
            "no valid segments: every candidate segment contained missing "
            "samples or the record is shorter than one segment"
        )

    used = stats.used
    full_p1 = [v / used for v in sum_p1]
    full_p2 = [v / used for v in sum_p2]
    full_cross = [v / used for v in sum_cross]

    # one-sided view
    n_one = nfft // 2 + 1
    has_nyquist = (nfft % 2) == 0
    freqs = [k * config.sample_rate / nfft for k in range(n_one)]
    psd1 = _one_sided(full_p1, n_one, has_nyquist)
    psd2 = _one_sided(full_p2, n_one, has_nyquist)
    cross = _one_sided(full_cross, n_one, has_nyquist)

    coherence: list[float | None] = []
    phase: list[float | None] = []
    for k in range(n_one):
        denom = psd1[k] * psd2[k]
        if denom == 0.0:
            coherence.append(None)
        else:
            mag2 = (cross[k].real ** 2 + cross[k].imag ** 2) / denom
            coherence.append(min(1.0, mag2))
        if cross[k] == 0j:
            phase.append(None)
        else:
            phase.append(math.atan2(cross[k].imag, cross[k].real))

    energy_check = _energy_consistency(
        full_p1, full_p2, energy_time, config.sample_rate, used, window_power
    )
    return SpectrumResult(
        frequencies=freqs,
        psd1=psd1,
        psd2=psd2,
        cross=cross,
        coherence=coherence,
        phase=phase,
        stats=stats,
        window_power=window_power,
        energy_check=energy_check,
    )


def _one_sided(values: list, n_one: int, has_nyquist: bool) -> list:
    out = list(values[:n_one])
    last = n_one - 1 if has_nyquist else n_one
    for k in range(1, last):
        out[k] = out[k] * 2.0
    return out


def _energy_consistency(
    full_p1: list[float],
    full_p2: list[float],
    energy_time: float,
    sample_rate: float,
    used: int,
    window_power: float,
) -> dict:
    """Compare mean spectral integral with mean windowed segment energy.

    Parseval: sum_k |X_k|^2 / nfft == sum_n |x_n|^2.  The averaged two-sided
    PSD integrates to mean_windowed_energy / sum(w^2); multiplying the
    spectral integral back by sum(w^2) must recover the time-domain energy.
    """
    nfft = len(full_p1)
    df = sample_rate / nfft
    spectral_integral = (sum(full_p1) + sum(full_p2)) * df * window_power
    time_integral = energy_time / used  # mean per-segment windowed energy
    # spectral integral equals time_integral / window_power by construction;
    # report both and their relative discrepancy as a consistency check.
    rel = 0.0
    if time_integral > 0.0:
        rel = abs(spectral_integral - time_integral) / time_integral
    return {
        "spectral_integral": spectral_integral,
        "mean_windowed_segment_energy": time_integral,
        "relative_difference": rel,
        "consistent": rel < 1e-9,
    }
