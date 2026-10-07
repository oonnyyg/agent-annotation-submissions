"""Read and validate mono 16-bit PCM WAV files."""

from __future__ import annotations

import wave
from dataclasses import dataclass


class WavFormatError(ValueError):
    """Raised when the input is not a mono 16-bit PCM WAV file."""


@dataclass(frozen=True)
class WavData:
    sample_rate: int
    samples: list[int]  # raw 16-bit signed samples


def read_wav(path: str) -> WavData:
    """Load a WAV file, enforcing mono 16-bit PCM."""
    try:
        with wave.open(path, "rb") as wav:
            channels = wav.getnchannels()
            width = wav.getsampwidth()
            comp = wav.getcomptype()
            rate = wav.getframerate()
            if comp != "NONE":
                raise WavFormatError(f"unsupported compression: {comp!r}")
            if channels != 1:
                raise WavFormatError(f"expected mono audio, got {channels} channels")
            if width != 2:
                raise WavFormatError(f"expected 16-bit samples, got {width * 8}-bit")
            frames = wav.readframes(wav.getnframes())
    except wave.Error as exc:
        raise WavFormatError(f"invalid WAV file: {exc}") from exc
    if rate <= 0:
        raise WavFormatError("invalid sample rate")
    count = len(frames) // 2
    samples = [
        int.from_bytes(frames[2 * i : 2 * i + 2], "little", signed=True)
        for i in range(count)
    ]
    return WavData(sample_rate=rate, samples=samples)
