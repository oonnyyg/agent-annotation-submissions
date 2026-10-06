"""命令行入口：python -m coincidence_replay"""
from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .core import ReplayError, find_coincidences, replay_channel
from .io import load_config, load_pulses
from .report import (
    build_summary,
    write_pairs_csv,
    write_pulse_status_csv,
    write_summary_json,
)


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="python -m coincidence_replay",
        description="双通道探测器回放：死时间分析与符合事件配对",
    )
    parser.add_argument("--pulses", required=True, help="脉冲 CSV 路径")
    parser.add_argument("--config", required=True, help="采集配置 JSON 路径")
    parser.add_argument("--out-dir", default="replay_out", help="输出目录")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        acq, channels = load_config(args.config)
        pulses = load_pulses(args.pulses, channels, acq)
    except ReplayError as exc:
        print(f"输入错误: {exc}", file=sys.stderr)
        return 2

    results = {
        name: replay_channel(pulses[name], cfg, acq)
        for name, cfg in channels.items()
    }
    name_a, name_b = sorted(channels)
    cfg_a, cfg_b = channels[name_a], channels[name_b]
    pairs, unpaired = find_coincidences(
        results[name_a].accepted_pulses,
        results[name_b].accepted_pulses,
        cfg_a.time_offset_ns,
        cfg_b.time_offset_ns,
        acq.coincidence_window_ns,
        acq,
    )

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    write_pulse_status_csv(results, out_dir / "pulse_status.csv")
    write_pairs_csv(
        pairs, unpaired,
        cfg_a.time_offset_ns, cfg_b.time_offset_ns,
        out_dir / "coincidence_pairs.csv",
    )
    summary = build_summary(acq, results, pairs, unpaired)
    write_summary_json(summary, out_dir / "summary.json")
    print(f"输出目录: {out_dir}")
    print(f"符合对数: {len(pairs)}，未配对: {len(unpaired)}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
