"""命令行入口：python -m ising_lab config.json --samples samples.csv --summary summary.json"""
from __future__ import annotations

import argparse
import sys

from .config import ConfigError, load_config
from .runner import run_experiment, write_samples_csv, write_summary_json


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="ising_lab",
        description="二维 Ising 模型 Metropolis 采样与小晶格精确枚举对比",
    )
    parser.add_argument("config", help="JSON 配置文件路径")
    parser.add_argument("--samples", default="samples.csv", help="样本 CSV 输出路径")
    parser.add_argument("--summary", default="summary.json", help="汇总 JSON 输出路径")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        config = load_config(args.config)
    except (ConfigError, OSError, ValueError) as exc:
        print(f"配置错误：{exc}", file=sys.stderr)
        return 2
    summary, results = run_experiment(config)
    write_samples_csv(args.samples, config, results)
    write_summary_json(args.summary, summary)
    for entry in summary["temperatures"]:
        line = (
            f"T={entry['temperature']:>8.4f}  "
            f"E/N={entry['sampled']['energy_per_spin']:+.6f}  "
            f"|m|={entry['sampled']['abs_magnetization_per_spin']:.6f}"
        )
        if "exact" in entry:
            line += f"  (精确 E/N={entry['exact']['energy_per_spin']:+.6f})"
        print(line)
    print(f"样本写入 {args.samples}，汇总写入 {args.summary}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
