"""命令行入口：python -m betfit data.csv config.json"""

from __future__ import annotations

import argparse
import sys

from .fit import FitResult, analyze_sample
from .io import BetInputError, load_config, load_samples


def _fmt(value: float | None, digits: int = 6) -> str:
    if value is None:
        return "n/a"
    return f"{value:.{digits}g}"


def render_report(results: list[FitResult]) -> str:
    lines: list[str] = []
    for res in results:
        s = res.sample
        lines.append(f"=== 样品 {s.name} ({len(s.pressures)} 点) ===")
        if res.selected is None:
            lines.append("结果: 失败 —— 没有通过筛选的区间")
        else:
            iv = res.selected
            p_lo = s.pressures[iv.start]
            p_hi = s.pressures[iv.end]
            lines.append(
                f"选中区间: p/p0 = [{_fmt(p_lo)}, {_fmt(p_hi)}], 点数 {iv.n_points}"
            )
            lines.append(f"  斜率            {_fmt(iv.slope)} g/mol")
            lines.append(f"  截距            {_fmt(iv.intercept)} g/mol")
            lines.append(f"  决定系数 R^2    {_fmt(iv.r_squared)}")
            lines.append(f"  单层容量 q_m    {_fmt(iv.monolayer_capacity)} mol/g")
            lines.append(f"  BET 常数 C      {_fmt(iv.bet_constant)}")
            lines.append(f"  比表面积        {_fmt(iv.surface_area)} m^2/g")
            lines.append("  逐点残差 (BET 纵坐标 p/(q*(1-p)), 单位 g/mol):")
            lines.append(f"    {'p/p0':>10} {'观测':>12} {'拟合':>12} {'残差':>12}")
            for p, y_obs, y_fit, r in res.residuals:
                lines.append(f"    {p:10.5f} {y_obs:12.5g} {y_fit:12.5g} {r:12.5g}")
        lines.append("  候选区间:")
        lines.append(
            f"    {'范围':>20} {'点数':>4} {'R^2':>10} {'C':>10} {'面积(m2/g)':>12}  状态"
        )
        for iv in res.candidates:
            p_lo = s.pressures[iv.start] if iv.end >= 0 else float("nan")
            p_hi = s.pressures[iv.end] if iv.end >= 0 else float("nan")
            status = "通过" if iv.accepted else "拒绝: " + "; ".join(iv.reasons)
            lines.append(
                f"    [{p_lo:8.5f},{p_hi:8.5f}] {iv.n_points:>4} "
                f"{_fmt(iv.r_squared):>10} {_fmt(iv.bet_constant):>10} "
                f"{_fmt(iv.surface_area):>12}  {status}"
            )
        lines.append("")
    lines.append(
        "说明: 上述筛选仅为本工具的拟合质量条件，不构成完整的吸附适用性认证。"
    )
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="betfit",
        description="BET 比表面积分析：自动或手动选择线性区间并输出可复核报告。",
    )
    parser.add_argument("csv", help="CSV：首列相对压力，其余列为各样品吸附量 (mol/g)")
    parser.add_argument("config", help="JSON：cross_section_nm2、min_points、intervals")
    parser.add_argument("-o", "--output", help="报告输出文件（默认打印到标准输出）")
    args = parser.parse_args(argv)

    try:
        samples = load_samples(args.csv)
        config = load_config(args.config)
    except BetInputError as exc:
        print(f"输入错误: {exc}", file=sys.stderr)
        return 2

    results = [analyze_sample(s, config) for s in samples]
    report = render_report(results)
    if args.output:
        with open(args.output, "w", encoding="utf-8") as fh:
            fh.write(report + "\n")
    else:
        print(report)
    return 0 if all(r.ok for r in results) else 1


if __name__ == "__main__":
    raise SystemExit(main())
