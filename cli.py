"""命令行入口：读取研究数据 CSV，输出 JSON/CSV 汇总报告。"""
from __future__ import annotations

import argparse
import sys

from meta_analysis import DataError, load_studies, run_meta_analysis
from report import write_csv, write_json


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="binary-meta",
        description="二分类结局 Meta 分析：固定/随机效应汇总与留一敏感性分析",
    )
    parser.add_argument("input", help="输入 CSV（study_id,exp_events,exp_nonevents,ctl_events,ctl_nonevents）")
    parser.add_argument("--json", dest="json_path", help="JSON 报告输出路径")
    parser.add_argument("--csv", dest="csv_path", help="CSV 报告输出路径")
    args = parser.parse_args(argv)

    if not args.json_path and not args.csv_path:
        parser.error("请至少指定 --json 或 --csv 一个输出")

    try:
        studies = load_studies(args.input)
        result = run_meta_analysis(studies)
    except DataError as exc:
        print(f"数据错误：{exc}", file=sys.stderr)
        return 2

    if args.json_path:
        write_json(result, args.json_path)
    if args.csv_path:
        write_csv(result, args.csv_path)

    print(
        f"纳入 {len(result.studies)} 项研究，排除 {len(result.excluded)} 项；"
        f"固定效应 OR={result.fixed.odds_ratio:.4f} "
        f"[{result.fixed.ci_lower:.4f}, {result.fixed.ci_upper:.4f}]；"
        f"随机效应 OR={result.random.odds_ratio:.4f} "
        f"[{result.random.ci_lower:.4f}, {result.random.ci_upper:.4f}]"
    )
    for excluded in result.excluded:
        print(f"排除 {excluded.study_id}：{excluded.reason}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
