"""Helpers to synthesize mono 16-bit PCM WAV files for tests."""

from __future__ import annotations

import math
import wave

from morse_decoder.morse import MORSE_TABLE

SAMPLE_RATE = 8000


def tone(duration: float, amplitude: float = 0.8, freq: float = 700.0) -> list[int]:
    n = round(duration * SAMPLE_RATE)
    return [
        int(amplitude * 32767 * math.sin(2 * math.pi * freq * i / SAMPLE_RATE))
        for i in range(n)
    ]


def silence(duration: float) -> list[int]:
    return [0] * round(duration * SAMPLE_RATE)


def encode_text(text: str, dot: float, amplitude: float = 0.8) -> list[int]:
    """Render text as Morse PCM samples with standard timing."""
    samples: list[int] = []
    for index, char in enumerate(text):
        if char == " ":
            samples += silence(4 * dot)  # 3 already emitted -> total 7
            continue
        code = MORSE_TABLE[char]
        for i, symbol in enumerate(code):
            samples += tone(dot if symbol == "." else 3 * dot, amplitude)
            samples += silence(dot)
        samples += silence(2 * dot)  # 1 already emitted -> total 3
    return samples


def write_wav(path: str, samples: list[int]) -> None:
    with wave.open(path, "wb") as wav:
        wav.setnchannels(1)
        wav.setsampwidth(2)
        wav.setframerate(SAMPLE_RATE)
        wav.writeframes(
            b"".join(s.to_bytes(2, "little", signed=True) for s in samples)
        )
