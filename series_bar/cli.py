"""命令行入口：python -m series_bar model.json displacements.csv --out-dir out"""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from .model import ModelError, load_model
from .report import (
    LoadHistoryError,
    build_diagnostics,
    load_displacement_history,
    write_diagnostics_json,
    write_segments_csv,
    write_steps_csv,
)
from .solver import SolverError, run_analysis


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="series_bar",
        description="串联杆件循环加载弹塑性分析（小应变一维，线性各向同性硬化）",
    )
    parser.add_argument("model", help="模型 JSON 文件路径")
    parser.add_argument("displacements", help="端部位移历程 CSV 文件路径")
    parser.add_argument("--out-dir", default="output", help="结果输出目录（默认 output）")
    parser.add_argument("--tol", type=float, default=1e-10, help="相对残差容差（默认 1e-10）")
    parser.add_argument("--max-iter", type=int, default=50, help="每步最大迭代次数（默认 50）")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        model = load_model(args.model)
        history = load_displacement_history(args.displacements)
        result = run_analysis(model, history, tolerance=args.tol, max_iterations=args.max_iter)
    except (ModelError, LoadHistoryError, SolverError) as exc:
        print(f"输入错误: {exc}", file=sys.stderr)
        return 2

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)
    write_steps_csv(result, model, out_dir / "steps.csv")
    write_segments_csv(result, model, out_dir / "segments.csv")
    diagnostics = build_diagnostics(result, model, args.tol, args.max_iter)
    write_diagnostics_json(diagnostics, out_dir / "diagnostics.json")

    converged_count = sum(1 for step in result.steps if step.converged)
    print(f"已完成 {converged_count}/{len(history)} 步，结果写入 {out_dir}")
    if not result.completed:
        print(f"第 {result.failed_step} 步未收敛，已停止并保留此前结果", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
