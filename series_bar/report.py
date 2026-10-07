"""位移历程 CSV 导入，以及 CSV 结果与 JSON 诊断输出。"""

from __future__ import annotations

import csv
import json
import math
from pathlib import Path

from .model import BarModel
from .solver import AnalysisResult


class LoadHistoryError(ValueError):
    """位移历程文件非法时抛出。"""


def load_displacement_history(path: str | Path) -> list[float]:
    """读取 CSV 位移历程；支持带表头（displacement 列）或单列无表头。"""
    try:
        text = Path(path).read_text(encoding="utf-8-sig")
    except OSError as exc:
        raise LoadHistoryError(f"无法读取位移文件 {path}: {exc}") from exc
    rows = [row for row in csv.reader(text.splitlines()) if row and any(c.strip() for c in row)]
    if not rows:
        raise LoadHistoryError("位移历程文件为空")

    try:
        float(rows[0][0].strip())
    except ValueError:
        header = [cell.strip().lower() for cell in rows[0]]
        if "displacement" not in header:
            raise LoadHistoryError(f"表头缺少 displacement 列: {rows[0]}") from None
        column = header.index("displacement")
        data_rows = rows[1:]
    else:
        column = 0
        data_rows = rows

    values: list[float] = []
    for row in data_rows:
        if column >= len(row):
            raise LoadHistoryError(f"行缺少位移字段: {row}")
        try:
            value = float(row[column].strip())
        except ValueError:
            raise LoadHistoryError(f"位移值无法解析为数值: {row[column]!r}") from None
        if not math.isfinite(value):
            raise LoadHistoryError(f"位移历程包含非有限值 {value!r}")
        values.append(value)
    if not values:
        raise LoadHistoryError("位移历程为空")
    return values


def _fmt(value: float | None) -> str:
    return "" if value is None else repr(value)


def write_steps_csv(result: AnalysisResult, model: BarModel, path: str | Path) -> None:
    """逐步汇总：反力、轴力、各节点位移与残差。"""
    node_columns = [f"u_{i}" for i in range(model.node_count)]
    header = [
        "step", "prescribed_displacement", "converged", "iterations",
        "reaction_force", "axial_force", *node_columns,
        "force_imbalance", "compatibility_residual",
    ]
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(header)
        for step in result.steps:
            nodes = step.node_displacements
            writer.writerow([
                step.step,
                repr(step.prescribed_displacement),
                "1" if step.converged else "0",
                step.iterations,
                _fmt(step.reaction_force),
                _fmt(step.axial_force),
                *([_fmt(v) for v in nodes] if nodes is not None else [""] * model.node_count),
                _fmt(step.force_imbalance),
                _fmt(step.compatibility_residual),
            ])


def write_segments_csv(result: AnalysisResult, model: BarModel, path: str | Path) -> None:
    """逐步各段应力应变与塑性历史明细。"""
    header = [
        "step", "segment", "strain", "stress", "axial_force",
        "plastic_strain", "accumulated_plastic_strain", "yielded",
    ]
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(header)
        for step in result.steps:
            if step.segments is None:
                continue
            for index, segment in enumerate(step.segments):
                writer.writerow([
                    step.step,
                    index,
                    repr(segment.strain),
                    repr(segment.stress),
                    repr(segment.axial_force),
                    repr(segment.plastic_strain),
                    repr(segment.accumulated_plastic_strain),
                    "1" if segment.yielded else "0",
                ])


def build_diagnostics(
    result: AnalysisResult,
    model: BarModel,
    tolerance: float,
    max_iterations: int,
) -> dict:
    """汇总 JSON 诊断：收敛信息、残差与最终塑性状态。"""
    last_converged = next(
        (step for step in reversed(result.steps) if step.converged), None
    )
    return {
        "model": model.name,
        "segment_count": len(model.segments),
        "total_steps": len(result.steps),
        "completed": result.completed,
        "failed_step": result.failed_step,
        "tolerance": tolerance,
        "max_iterations": max_iterations,
        "steps": [
            {
                "step": step.step,
                "prescribed_displacement": step.prescribed_displacement,
                "converged": step.converged,
                "iterations": step.iterations,
                "reaction_force": step.reaction_force,
                "axial_force": step.axial_force,
                "force_imbalance": step.force_imbalance,
                "compatibility_residual": step.compatibility_residual,
            }
            for step in result.steps
        ],
        "final_state": (
            {
                "step": last_converged.step,
                "node_displacements": list(last_converged.node_displacements),
                "segments": [
                    {
                        "plastic_strain": seg.plastic_strain,
                        "accumulated_plastic_strain": seg.accumulated_plastic_strain,
                        "stress": seg.stress,
                        "strain": seg.strain,
                    }
                    for seg in last_converged.segments
                ],
            }
            if last_converged is not None
            else None
        ),
    }


def write_diagnostics_json(diagnostics: dict, path: str | Path) -> None:
    Path(path).write_text(
        json.dumps(diagnostics, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
