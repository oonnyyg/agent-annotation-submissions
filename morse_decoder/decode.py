"""Turn voiced/silent segments into Morse text with strict tolerance checks."""

from __future__ import annotations

from dataclasses import dataclass, field

from .morse import CODE_TO_CHAR
from .segment import Segment


@dataclass
class DecodeError:
    start: float
    end: float
    reason: str


@dataclass
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


def classify_tone(units: float, tolerance: float) -> str | None:
    """Return '.' or '-' if units match uniquely within tolerance."""
    dot_ok = abs(units - 1.0) <= tolerance
    dash_ok = abs(units - 3.0) <= tolerance
    if dot_ok and dash_ok:
        return None
    if dot_ok:
        return "."
    if dash_ok:
        return "-"
    return None


def classify_gap(units: float, tolerance: float) -> str | None:
    """Return 'intra', 'char' or 'word' for a silence, or None if invalid."""
    if units >= 7.0 - tolerance:
        return "word"
    if abs(units - 3.0) <= tolerance:
        return "char"
    if abs(units - 1.0) <= tolerance:
        return "intra"
    return None


def decode_segments(segments: list[Segment], dot_s: float,
                    tolerance: float) -> DecodeResult:
    """Decode segments; leading/trailing silence ignored, final char flushed."""
    result = DecodeResult(text="")
    segs = [s for s in segments]
    while segs and not segs[0].voiced:
        segs.pop(0)
    while segs and not segs[-1].voiced:
        segs.pop()

    code = ""
    char_start = 0.0
    char_end = 0.0

    def flush() -> None:
        nonlocal code, char_start
        if not code:
            return
        char = CODE_TO_CHAR.get(code)
        if char is None:
            result.errors.append(DecodeError(
                char_start, char_end, f"未知码组 {code!r}"))
        else:
            result.characters.append(
                CharRecord(char, code, char_start, char_end))
        code = ""

    for seg in segs:
        units = (seg.end - seg.start) / dot_s
        if seg.voiced:
            mark = classify_tone(units, tolerance)
            if mark is None:
                result.errors.append(DecodeError(
                    seg.start, seg.end,
                    f"有声时长 {units:.2f} 单元无法在容差内唯一解释为点或划"))
                flush()
                continue
            if not code:
                char_start = seg.start
            code += mark
            char_end = seg.end
        else:
            gap = classify_gap(units, tolerance)
            if gap is None:
                result.errors.append(DecodeError(
                    seg.start, seg.end,
                    f"静音时长 {units:.2f} 单元无法解释为码元内/字符/词间隔"))
                flush()
                continue
            if gap == "intra":
                continue
            flush()
            if gap == "word":
                result.characters.append(
                    CharRecord(" ", "", seg.start, seg.end))
    flush()

    if result.errors:
        result.text = ""
    else:
        result.text = "".join(c.char for c in result.characters)
    return result
