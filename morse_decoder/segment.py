"""RMS envelope segmentation with hysteresis thresholds."""

from __future__ import annotations

from dataclasses import dataclass

from .wavfile import PcmAudio


@dataclass
class Segment:
    voiced: bool
    start: float
    end: float


def window_rms(samples, start, end):
    chunk = samples[start:end]
    if not chunk:
        return 0.0
    return (sum(v * v for v in chunk) / len(chunk)) ** 0.5


def segment_audio(audio: PcmAudio, window_s: float, threshold_on: float,
                  threshold_off: float) -> list[Segment]:
    """Split audio into voiced/silent segments with hysteresis.

    RMS >= threshold_on switches to voiced, RMS < threshold_off switches
    to silent, values in between keep the previous state. The final
    partial window is evaluated too; equal-state windows are merged.
    """
    if not 0.0 <= threshold_off < threshold_on <= 1.0:
        raise ValueError(
            "阈值配置非法: 需要 0 <= 停声阈值 < 启声阈值 <= 1, "
            f"得到 off={threshold_off}, on={threshold_on}"
        )
    win = max(1, round(window_s * audio.sample_rate))
    total = len(audio.samples)
    segments: list[Segment] = []
    voiced = False
    seg_start = 0
    pos = 0
    while pos < total:
        end = min(pos + win, total)
        rms = window_rms(audio.samples, pos, end)
        if rms >= threshold_on:
            new_state = True
        elif rms < threshold_off:
            new_state = False
        else:
            new_state = voiced
        if new_state != voiced:
            if pos > seg_start:
                segments.append(Segment(voiced, seg_start / audio.sample_rate,
                                        pos / audio.sample_rate))
            voiced = new_state
            seg_start = pos
        pos = end
    segments.append(Segment(voiced, seg_start / audio.sample_rate,
                            total / audio.sample_rate))
    return segments
