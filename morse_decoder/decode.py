"""Turn tone/silence segments into Morse text with a timeline."""

from __future__ import annotations

from dataclasses import dataclass, field

from .morse import CODE_TO_CHAR
from .segment import Segment


@dataclass(frozen=True)
class DecodeConfig:
    dot_seconds: float
    tolerance: float  # relative to the dot duration, 0 <= tolerance < 1


@dataclass(frozen=True)
class DecodeError:
    start: float
    end: float
    reason: str


@dataclass(frozen=True)
class CharRecord:
    char: str
    code: str
    start: float
    end: float


@dataclass
class DecodeResult:
    text: str
    characters: list[CharRecord] = field(default_factory=list)
    errors: list[DecodeError] = field(default_factory=list)


def validate_decode_config(cfg: DecodeConfig) -> None:
    if cfg.dot_seconds <= 0:
        raise ValueError("dot_seconds must be positive")
    if not (0.0 <= cfg.tolerance < 1.0):
        raise ValueError("tolerance must satisfy 0 <= tolerance < 1")


def _classify(duration: float, units: list[int], cfg: DecodeConfig) -> int | None:
    """Return the unique matching unit count, or None if ambiguous/unmatched."""
    matches = [
        u for u in units
        if abs(duration - u * cfg.dot_seconds) <= cfg.tolerance * cfg.dot_seconds
    ]
    return matches[0] if len(matches) == 1 else None


def decode_segments(segments: list[Segment], cfg: DecodeConfig) -> DecodeResult:
    """Decode merged segments; leading/trailing silence is ignored."""
    result = DecodeResult(text="")
    code = ""
    char_start: float | None = None
    char_end = 0.0

    def flush_char() -> None:
        nonlocal code, char_start
        if not code:
            return
        char = CODE_TO_CHAR.get(code)
        if char is None:
            result.errors.append(
                DecodeError(char_start, char_end, f"unknown Morse code {code!r}")
            )
        else:
            result.characters.append(CharRecord(char, code, char_start, char_end))
        code = ""
        char_start = None

    for seg in segments:
        duration = seg.end - seg.start
        if seg.is_tone:
            unit = _classify(duration, [1, 3], cfg)
            if unit is None:
                result.errors.append(
                    DecodeError(seg.start, seg.end,
                                f"tone duration {duration:.4f}s is ambiguous")
                )
                continue
            if char_start is None:
                char_start = seg.start
            code += "." if unit == 1 else "-"
            char_end = seg.end
        else:
            if char_start is None:
                continue  # leading silence
            unit = _classify(duration, [1, 3], cfg)
            if unit is None and duration >= (7 - cfg.tolerance) * cfg.dot_seconds:
                unit = 7  # word gap is at least seven units
            if unit is None:
                result.errors.append(
                    DecodeError(seg.start, seg.end,
                                f"silence duration {duration:.4f}s is ambiguous")
                )
                continue
            if unit == 3:
                flush_char()
            elif unit == 7:
                flush_char()
                result.characters.append(
                    CharRecord(" ", " ", seg.start, seg.end)
                )
    flush_char()  # settle the final character at end of file
    if result.errors:
        result.text = ""
        return result
    result.text = "".join(c.char for c in result.characters)
    return result
