"""Command line interface for the Morse audio decoder."""

from __future__ import annotations

import argparse
import json
import os
import sys

from .decode import DecodeConfig, decode_segments, validate_decode_config
from .segment import (
    SegmentConfig,
    normalize_samples,
    segment_audio,
    validate_segment_config,
)
from .wavio import read_wav


def _load_config(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as fh:
        cfg = json.load(fh)
    if not isinstance(cfg, dict):
        raise ValueError("configuration JSON must be an object")
    return cfg


def _write_new(path: str, content: str) -> None:
    if os.path.exists(path):
        raise FileExistsError(f"refusing to overwrite existing file: {path}")
    with open(path, "x", encoding="utf-8") as fh:
        fh.write(content)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="morse-decoder",
        description="Decode Morse code from a mono 16-bit PCM WAV file.",
    )
    parser.add_argument("wav", help="input WAV file (mono, 16-bit PCM)")
    parser.add_argument("config", help="JSON parameter file")
    parser.add_argument("--segments-out", required=True, help="JSON diagnostics output")
    parser.add_argument("--text-out", required=True, help="decoded text output")
    args = parser.parse_args(argv)

    try:
        raw = _load_config(args.config)
        seg_cfg = SegmentConfig(
            window_seconds=float(raw["window_seconds"]),
            onset_threshold=float(raw["onset_threshold"]),
            offset_threshold=float(raw["offset_threshold"]),
        )
        dec_cfg = DecodeConfig(
            dot_seconds=float(raw["dot_seconds"]),
            tolerance=float(raw["tolerance"]),
        )
        validate_decode_config(dec_cfg)
        validate_segment_config(seg_cfg, dec_cfg.dot_seconds)
        wav = read_wav(args.wav)
    except (KeyError, ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    samples = normalize_samples(wav.samples)
    segments = segment_audio(samples, wav.sample_rate, seg_cfg)
    result = decode_segments(segments, dec_cfg)

    diagnostics = {
        "sample_rate": wav.sample_rate,
        "config": {
            "window_seconds": seg_cfg.window_seconds,
            "onset_threshold": seg_cfg.onset_threshold,
            "offset_threshold": seg_cfg.offset_threshold,
            "dot_seconds": dec_cfg.dot_seconds,
            "tolerance": dec_cfg.tolerance,
        },
        "segments": [
            {"type": "tone" if s.is_tone else "silence",
             "start": round(s.start, 6), "end": round(s.end, 6)}
            for s in segments
        ],
        "characters": [
            {"char": c.char, "code": c.code,
             "start": round(c.start, 6), "end": round(c.end, 6)}
            for c in result.characters
        ],
        "errors": [
            {"start": round(e.start, 6), "end": round(e.end, 6),
             "reason": e.reason}
            for e in result.errors
        ],
        "text": result.text,
    }
    try:
        _write_new(args.segments_out, json.dumps(diagnostics, indent=2) + "\n")
        _write_new(args.text_out, result.text + "\n")
    except OSError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    if result.errors:
        print(f"decode failed with {len(result.errors)} error(s)", file=sys.stderr)
        return 1
    print(result.text)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
