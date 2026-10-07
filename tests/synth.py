"""Helpers to synthesize mono 16-bit PCM WAV files for tests."""

from __future__ import annotations

import math
import struct
import wave

SAMPLE_RATE = 8000
TONE_HZ = 700.0


def tone(seconds: float, amplitude: float = 0.8,
         rate: int = SAMPLE_RATE) -> list[float]:
    n = int(round(seconds * rate))
    return [amplitude * math.sin(2.0 * math.pi * TONE_HZ * i / rate)
            for i in range(n)]


def silence(seconds: float, rate: int = SAMPLE_RATE) -> list[float]:
    return [0.0] * int(round(seconds * rate))


def encode_text(text: str, dot: float, table: dict,
                amplitude: float = 0.8) -> list[float]:
    """Render text as samples using unit timing (dot=1, dash=3, etc.)."""
    out: list[float] = []
    for idx, char in enumerate(text):
        if char == " ":
            out += silence(7 * dot)
            continue
        if idx > 0 and text[idx - 1] != " ":
            out += silence(3 * dot)
        code = table[char]
        for j, mark in enumerate(code):
            if j > 0:
                out += silence(1 * dot)
            out += tone((1 if mark == "." else 3) * dot, amplitude)
    return out


def write_wav(path: str, samples: list[float],
              rate: int = SAMPLE_RATE) -> None:
    frames = b"".join(
        struct.pack("<h", max(-32768, min(32767, int(round(v * 32767)))))
        for v in samples)
    with wave.open(path, "wb") as wav:
        wav.setnchannels(1)
        wav.setsampwidth(2)
        wav.setframerate(rate)
        wav.writeframes(frames)
