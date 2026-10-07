"""Split normalized audio into tone/silence segments with hysteresis."""

from __future__ import annotations

import math
from dataclasses import dataclass


class ConfigError(ValueError):
    """Raised for invalid decoder configuration."""


@dataclass(frozen=True)
class Segment:
    is_tone: bool
    start: float
    end: float


@dataclass(frozen=True)
class SegmentConfig:
    window_seconds: float
    onset_threshold: float
    offset_threshold: float


def validate_segment_config(cfg: SegmentConfig, dot_seconds: float) -> None:
    if cfg.window_seconds <= 0:
        raise ConfigError("window_seconds must be positive")
    if cfg.window_seconds > dot_seconds / 5:
        raise ConfigError(
            "window_seconds must not exceed one fifth of the dot duration"
        )
    if not (0.0 <= cfg.offset_threshold < cfg.onset_threshold <= 1.0):
        raise ConfigError(
            "thresholds must satisfy 0 <= offset_threshold < onset_threshold <= 1"
        )


def normalize_samples(samples: list[int]) -> list[float]:
    return [max(-1.0, s / 32768.0) for s in samples]


def segment_audio(
    samples: list[float], sample_rate: int, cfg: SegmentConfig
) -> list[Segment]:
    window = max(1, round(cfg.window_seconds * sample_rate))
    segments: list[Segment] = []
    is_tone = False
    seg_start = 0.0
    pos = 0
    total = len(samples)

    def emit(tone_flag: bool, start: float, end: float) -> None:
        if end > start:
            segments.append(Segment(tone_flag, start, end))

    while pos < total:
        end = min(pos + window, total)
        chunk = samples[pos:end]
        rms = math.sqrt(sum(v * v for v in chunk) / len(chunk))
        if is_tone:
            if rms < cfg.offset_threshold:
                emit(True, seg_start, pos / sample_rate)
                is_tone = False
                seg_start = pos / sample_rate
        else:
            if rms > cfg.onset_threshold:
                emit(False, seg_start, pos / sample_rate)
                is_tone = True
                seg_start = pos / sample_rate
        pos = end
    emit(is_tone, seg_start, total / sample_rate)
    return segments
