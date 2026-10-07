"""Minimal mono 16-bit PCM WAV reader (standard library only)."""

from __future__ import annotations

import struct
import wave
from dataclasses import dataclass

FULL_SCALE = 32768.0


class AudioFormatError(ValueError):
    """Raised when the input is not a mono 16-bit PCM WAV file."""


@dataclass
class PcmAudio:
    sample_rate: int
    samples: list[float]  # normalized to [-1.0, 1.0] by full scale


def load_wav(path: str) -> PcmAudio:
    try:
        ctx = wave.open(path, "rb")
    except (wave.Error, EOFError, OSError) as exc:
        raise AudioFormatError(f"无法读取 WAV 文件 {path!r}: {exc}") from exc
    with ctx as wav:
        channels = wav.getnchannels()
        width = wav.getsampwidth()
        comptype = wav.getcomptype()
        rate = wav.getframerate()
        if comptype != "NONE":
            raise AudioFormatError(f"不支持的压缩格式: {comptype!r}，仅接受 PCM")
        if channels != 1:
            raise AudioFormatError(f"声道数为 {channels}，仅接受单声道")
        if width != 2:
            raise AudioFormatError(f"样本宽度为 {width * 8} 位，仅接受 16 位 PCM")
        frames = wav.readframes(wav.getnframes())
    count = len(frames) // 2
    ints = struct.unpack(f"<{count}h", frames[: count * 2])
    return PcmAudio(rate, [v / FULL_SCALE for v in ints])
