"""Command line interface for the raking tool."""

from __future__ import annotations

import argparse
import sys

from .core import RakingError, load_samples, load_targets, rake
from .output import write_failure, write_success


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="raking",
        description="按已知总体边际对调查样本权重做迭代比例调整（raking）",
    )
    parser.add_argument("--samples", required=True, help="样本 CSV 路径")
    parser.add_argument("--targets", required=True, help="目标边际 JSON 路径")
    parser.add_argument("--out", required=True, help="输出目录")
    parser.add_argument("--id-field", default="id", help="样本标识列名（默认 id）")
    parser.add_argument("--weight-field", default="weight", help="初始权重列名（默认 weight）")
    parser.add_argument("--tolerance", type=float, default=1e-6, help="收敛容差（默认 1e-6）")
    parser.add_argument("--max-rounds", type=int, default=100, help="最大迭代轮数（默认 100）")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        ids, rows, initial_weights = load_samples(args.samples, args.id_field, args.weight_field)
        margins = load_targets(args.targets)
        result = rake(
            rows,
            initial_weights,
            margins,
            tolerance=args.tolerance,
            max_rounds=args.max_rounds,
        )
    except RakingError as exc:
        print(f"输入错误: {exc}", file=sys.stderr)
        return 2

    if result.converged:
        weights_path, margins_path = write_success(args.out, ids, initial_weights, margins, result)
        print(f"收敛于第 {len(result.rounds)} 轮，有效样本量 {result.effective_sample_size:.4f}")
        print(f"权重文件: {weights_path}")
        print(f"边际对照: {margins_path}")
        return 0
    diag_path = write_failure(args.out, margins, result)
    print(f"未收敛: {result.failure_reason}", file=sys.stderr)
    print(f"诊断文件: {diag_path}", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
