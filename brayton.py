"""回热布雷顿循环教学核算器（纯 Python 标准库）。

仅用于热力学教学演示，不构成任何实际发动机设计建议。
"""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys
from dataclasses import dataclass, field


@dataclass(frozen=True)
class CycleConfig:
    """循环固定参数（JSON 输入）。"""

    compressor_inlet_temp: float  # T1, K
    turbine_inlet_temp: float  # T3, K
    specific_heat_cp: float  # kJ/(kg*K)
    heat_capacity_ratio: float  # gamma > 1
    compressor_efficiency: float  # 0 < eta <= 1
    turbine_efficiency: float  # 0 < eta <= 1
    regenerator_effectiveness: float  # 0 <= eps <= 1


@dataclass(frozen=True)
class CaseResult:
    """单个压比工况的核算结果。"""

    pressure_ratio: float
    t1: float
    t2: float  # 压缩机实际出口
    t3: float  # 涡轮入口
    t4: float  # 涡轮实际出口
    t2_ideal: float
    t4_ideal: float
    regenerator_heat: float  # 回热量 q_regen, kJ/kg
    compressor_work: float  # w_c, kJ/kg
    turbine_work: float  # w_t, kJ/kg
    net_work: float  # w_net, kJ/kg
    heat_in: float  # 外部吸热 q_in, kJ/kg
    heat_out: float  # 外部排热 q_out, kJ/kg
    thermal_efficiency: float | None
    usable: bool
    notes: list[str] = field(default_factory=list)


def _require_finite(name: str, value: float) -> float:
    value = float(value)
    if not math.isfinite(value):
        raise ValueError(f"{name} 必须为有限数值，得到 {value!r}")
    return value


def validate_config(raw: dict) -> CycleConfig:
    """校验并构造循环参数，非法输入抛出 ValueError。"""

    required = (
        "compressor_inlet_temp",
        "turbine_inlet_temp",
        "specific_heat_cp",
        "heat_capacity_ratio",
        "compressor_efficiency",
        "turbine_efficiency",
        "regenerator_effectiveness",
    )
    missing = [k for k in required if k not in raw]
    if missing:
        raise ValueError(f"JSON 缺少字段: {', '.join(missing)}")
    values = {k: _require_finite(k, raw[k]) for k in required}
    cfg = CycleConfig(**values)
    if cfg.compressor_inlet_temp <= 0:
        raise ValueError("压缩机入口温度必须为正（开尔文）")
    if cfg.turbine_inlet_temp <= cfg.compressor_inlet_temp:
        raise ValueError("涡轮入口温度必须高于压缩机入口温度")
    if cfg.specific_heat_cp <= 0:
        raise ValueError("定压比热必须为正常数")
    if cfg.heat_capacity_ratio <= 1:
        raise ValueError("比热比必须大于一")
    if not 0 < cfg.compressor_efficiency <= 1:
        raise ValueError("压缩机等熵效率须在 (0, 1] 内")
    if not 0 < cfg.turbine_efficiency <= 1:
        raise ValueError("涡轮等熵效率须在 (0, 1] 内")
    if not 0 <= cfg.regenerator_effectiveness <= 1:
        raise ValueError("回热器有效度须在 [0, 1] 内")
    return cfg


def parse_pressure_ratios(rows: list[dict]) -> list[float]:
    """从 CSV 行解析互不重复且大于一的压比。"""

    ratios: list[float] = []
    for i, row in enumerate(rows, start=1):
        if "pressure_ratio" not in row or row["pressure_ratio"] in (None, ""):
            raise ValueError(f"CSV 第 {i} 行缺少 pressure_ratio 字段")
        try:
            r = _require_finite("pressure_ratio", float(row["pressure_ratio"]))
        except ValueError:
            raise ValueError(f"CSV 第 {i} 行压比非法: {row['pressure_ratio']!r}") from None
        if r <= 1:
            raise ValueError(f"CSV 第 {i} 行压比必须大于一，得到 {r}")
        if r in ratios:
            raise ValueError(f"CSV 压比重复: {r}")
        ratios.append(r)
    if not ratios:
        raise ValueError("CSV 中没有任何工况")
    return ratios


def evaluate_case(cfg: CycleConfig, pressure_ratio: float) -> CaseResult:
    """核算单工况：先理想等熵温度，再按各自效率修正实际温度。"""

    gamma = cfg.heat_capacity_ratio
    cp = cfg.specific_heat_cp
    exponent = (gamma - 1.0) / gamma

    t1 = cfg.compressor_inlet_temp
    t3 = cfg.turbine_inlet_temp
    # 理想等熵出口温度（膨胀比等于压缩比，忽略所有压降）。
    t2_ideal = t1 * pressure_ratio**exponent
    t4_ideal = t3 * pressure_ratio ** (-exponent)
    # 压缩机效率作用于温升分母；涡轮效率作用于理想温降（分子）。
    t2 = t1 + (t2_ideal - t1) / cfg.compressor_efficiency
    t4 = t3 - cfg.turbine_efficiency * (t3 - t4_ideal)

    notes: list[str] = []
    # 仅当涡轮排气比压缩机出口热时才回热，否则旁路。
    if t4 > t2:
        q_regen = cfg.regenerator_effectiveness * cp * (t4 - t2)
    else:
        q_regen = 0.0
        notes.append("涡轮排气不高于压缩机出口，回热器旁路，回热量为零")

    w_c = cp * (t2 - t1)
    w_t = cp * (t3 - t4)
    w_net = w_t - w_c
    # 回热后冷侧温度与热侧回热后温度。
    t2_after = t2 + q_regen / cp
    t4_after = t4 - q_regen / cp
    q_in = cp * (t3 - t2_after)
    q_out = cp * (t4_after - t1)

    usable = True
    if q_in <= 0:
        usable = False
        notes.append("外部吸热非正，工况不可用")
    if w_net <= 0:
        usable = False
        notes.append("净功非正，工况不可用")
    efficiency = w_net / q_in if q_in > 0 else None

    return CaseResult(
        pressure_ratio=pressure_ratio,
        t1=t1, t2=t2, t3=t3, t4=t4,
        t2_ideal=t2_ideal, t4_ideal=t4_ideal,
        regenerator_heat=q_regen,
        compressor_work=w_c, turbine_work=w_t, net_work=w_net,
        heat_in=q_in, heat_out=q_out,
        thermal_efficiency=efficiency,
        usable=usable, notes=notes,
    )


def select_best(results: list[CaseResult]) -> tuple[CaseResult | None, CaseResult | None]:
    """在可用工况中选净功最大与效率最高者，同值取较小压比。"""

    usable = [r for r in results if r.usable]
    if not usable:
        return None, None
    best_work = min(usable, key=lambda r: (-r.net_work, r.pressure_ratio))
    best_eff = min(
        usable,
        key=lambda r: (-(r.thermal_efficiency or 0.0), r.pressure_ratio),
    )
    return best_work, best_eff


def check_energy_balance(result: CaseResult, tol: float = 1e-9) -> bool:
    """核对净功等于外部吸热减排热。"""

    return abs(result.net_work - (result.heat_in - result.heat_out)) <= tol


def format_report(results: list[CaseResult]) -> str:
    lines = ["回热布雷顿循环核算结果（教学用途，非发动机设计建议）", ""]
    for r in results:
        eff = f"{r.thermal_efficiency:.4f}" if r.thermal_efficiency is not None else "N/A"
        lines.append(f"压比 {r.pressure_ratio:g}:")
        lines.append(
            f"  温度 K: T1={r.t1:.2f} T2={r.t2:.2f} T3={r.t3:.2f} T4={r.t4:.2f}"
            f" (理想 T2s={r.t2_ideal:.2f} T4s={r.t4_ideal:.2f})"
        )
        lines.append(
            f"  比功 kJ/kg: 压缩 {r.compressor_work:.3f} 涡轮 {r.turbine_work:.3f}"
            f" 净功 {r.net_work:.3f}"
        )
        lines.append(
            f"  热量 kJ/kg: 回热 {r.regenerator_heat:.3f}"
            f" 外部吸热 {r.heat_in:.3f} 外部排热 {r.heat_out:.3f}"
        )
        lines.append(f"  热效率: {eff}  可用: {'是' if r.usable else '否'}")
        balance = "通过" if check_energy_balance(r) else "不通过"
        lines.append(f"  能量核对 (净功=吸热-排热): {balance}")
        for note in r.notes:
            lines.append(f"  诊断: {note}")
        lines.append("")
    best_work, best_eff = select_best(results)
    if best_work is None:
        lines.append("无可用工况：所有压比下外部吸热或净功均非正。")
    else:
        lines.append(
            f"净功最大方案: 压比 {best_work.pressure_ratio:g},"
            f" 净功 {best_work.net_work:.3f} kJ/kg"
        )
        lines.append(
            f"效率最高方案: 压比 {best_eff.pressure_ratio:g},"
            f" 热效率 {best_eff.thermal_efficiency:.4f}"
        )
    return "\n".join(lines)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="回热布雷顿循环教学核算器（不提供实际发动机设计建议）"
    )
    parser.add_argument("config", help="循环参数 JSON 文件路径")
    parser.add_argument("cases", help="压比工况 CSV 文件路径（列名 pressure_ratio）")
    args = parser.parse_args(argv)

    try:
        with open(args.config, encoding="utf-8") as fh:
            cfg = validate_config(json.load(fh))
        with open(args.cases, encoding="utf-8", newline="") as fh:
            ratios = parse_pressure_ratios(list(csv.DictReader(fh)))
    except (OSError, json.JSONDecodeError, ValueError) as exc:
        print(f"输入错误: {exc}", file=sys.stderr)
        return 2

    results = [evaluate_case(cfg, r) for r in ratios]
    print(format_report(results))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
