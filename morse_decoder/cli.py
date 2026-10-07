"""Command line interface: decode Morse from a mono 16-bit PCM WAV file."""

from __future__ import annotations

import argparse
import dataclasses
import json
import os
import sys

from .decode import decode_segments
from .segment import segment_audio
from .wavfile import AudioFormatError, load_wav


def load_config(path: str) -> dict:
    with open(path, "r", encoding="utf-8") as fh:
        cfg = json.load(fh)
    required = ["dot_duration", "tolerance", "window",
                "threshold_on", "threshold_off"]
    missing = [k for k in required if k not in cfg]
    if missing:
        raise ValueError(f"配置缺少字段: {', '.join(missing)}")
    for key in required:
        if not isinstance(cfg[key], (int, float)) or cfg[key] < 0:
            raise ValueError(f"配置项 {key} 必须为非负数值")
    if cfg["dot_duration"] <= 0:
        raise ValueError("dot_duration 必须大于 0")
    if cfg["window"] <= 0:
        raise ValueError("window 必须大于 0")
    if cfg["window"] > cfg["dot_duration"] / 5.0:
        raise ValueError(
            f"窗长 {cfg['window']}s 不得大于点时长五分之一 "
            f"({cfg['dot_duration'] / 5.0}s)")
    if not 0.0 <= cfg["threshold_off"] < cfg["threshold_on"] <= 1.0:
        raise ValueError("阈值需满足 0 <= threshold_off < threshold_on <= 1")
    return cfg


def build_report(cfg: dict, segments, result) -> dict:
    return {
        "config": cfg,
        "segments": [dataclasses.asdict(s) for s in segments],
        "characters": [dataclasses.asdict(c) for c in result.characters],
        "errors": [dataclasses.asdict(e) for e in result.errors],
        "text": result.text if not result.errors else None,
    }


def write_json(path: str, payload: dict) -> None:
    if os.path.exists(path):
        raise FileExistsError(f"输出文件已存在，拒绝覆盖: {path}")
    with open(path, "x", encoding="utf-8") as fh:
        json.dump(payload, fh, ensure_ascii=False, indent=2)
        fh.write("\n")


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="morse-decoder",
        description="从单声道 16 位 PCM WAV 中解码摩尔斯电文（仅标准库）")
    parser.add_argument("audio", help="输入 WAV 文件路径")
    parser.add_argument("config", help="JSON 参数文件路径")
    parser.add_argument("-o", "--output", required=True,
                        help="诊断与结果 JSON 输出路径（不得已存在）")
    args = parser.parse_args(argv)

    try:
        cfg = load_config(args.config)
        audio = load_wav(args.audio)
        segments = segment_audio(audio, cfg["window"],
                                 cfg["threshold_on"], cfg["threshold_off"])
        result = decode_segments(segments, cfg["dot_duration"],
                                 cfg["tolerance"])
        report = build_report(cfg, segments, result)
        write_json(args.output, report)
    except (AudioFormatError, ValueError, FileExistsError, OSError,
            json.JSONDecodeError) as exc:
        print(f"错误: {exc}", file=sys.stderr)
        return 2

    if result.errors:
        print(f"解码失败，共 {len(result.errors)} 处错误，详见 {args.output}",
              file=sys.stderr)
        return 1
    print(result.text)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
