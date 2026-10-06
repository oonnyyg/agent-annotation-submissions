"""命令行入口：python -m coincidence_replay <pulses.csv> <config.json> [-o out_dir]。"""

from __future__ import annotations

import argparse
import sys

from .model import ReplayError, load_config, load_pulses
from .report import run_replay, write_outputs


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="coincidence_replay",
        description="双通道探测器回放：死时间分析与符合配对",
    )
    parser.add_argument("pulses_csv", help="脉冲 CSV（channel,event_id,time_ns）")
    parser.add_argument("config_json", help="JSON 采集配置")
    parser.add_argument("-o", "--out-dir", default="replay_out", help="输出目录")
    args = parser.parse_args(argv)
    try:
        config = load_config(args.config_json)
        pulses = load_pulses(args.pulses_csv, config)
    except (ReplayError, OSError) as exc:
        print(f"错误: {exc}", file=sys.stderr)
        return 2
    result = run_replay(pulses, config)
    write_outputs(result, args.out_dir)
    print(f"符合对数: {result.summary['coincidence_count']}，输出目录: {args.out_dir}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
