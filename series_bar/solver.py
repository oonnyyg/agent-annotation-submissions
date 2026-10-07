"""全局平衡迭代：一致切线 Newton 法，试算与提交状态严格分离。"""

from __future__ import annotations

import math
from dataclasses import dataclass, field

from .material import MaterialPointResult, MaterialState, return_mapping
from .model import BarModel


class SolverError(ValueError):
    """求解器参数非法时抛出。"""


@dataclass(frozen=True)
class SegmentResult:
    """收敛步中单个杆段的明细。"""

    strain: float
    stress: float
    axial_force: float
    plastic_strain: float
    accumulated_plastic_strain: float
    yielded: bool


@dataclass(frozen=True)
class StepResult:
    """一个加载步的结果；未收敛时结果字段为 None 而非零值。"""

    step: int
    prescribed_displacement: float
    converged: bool
    iterations: int
    reaction_force: float | None
    axial_force: float | None
    node_displacements: tuple[float, ...] | None
    segments: tuple[SegmentResult, ...] | None
    force_imbalance: float | None
    compatibility_residual: float | None


@dataclass
class AnalysisResult:
    """整段位移历程的分析结果。"""

    steps: list[StepResult] = field(default_factory=list)
    failed_step: int | None = None

    @property
    def completed(self) -> bool:
        return self.failed_step is None


def _solve_dense(matrix: list[list[float]], rhs: list[float]) -> list[float]:
    """部分主元高斯消元，求解中小规模稠密线性系统。"""
    size = len(rhs)
    for col in range(size):
        pivot = max(range(col, size), key=lambda r: abs(matrix[r][col]))
        if abs(matrix[pivot][col]) == 0.0:
            raise SolverError("切线刚度矩阵奇异，无法继续迭代")
        if pivot != col:
            matrix[col], matrix[pivot] = matrix[pivot], matrix[col]
            rhs[col], rhs[pivot] = rhs[pivot], rhs[col]
        for row in range(col + 1, size):
            factor = matrix[row][col] / matrix[col][col]
            if factor == 0.0:
                continue
            for k in range(col, size):
                matrix[row][k] -= factor * matrix[col][k]
            rhs[row] -= factor * rhs[col]
    solution = [0.0] * size
    for row in range(size - 1, -1, -1):
        accum = rhs[row] - sum(matrix[row][k] * solution[k] for k in range(row + 1, size))
        solution[row] = accum / matrix[row][row]
    return solution


def _evaluate(
    model: BarModel,
    committed: list[MaterialState],
    displacements: list[float],
) -> tuple[list[MaterialPointResult], list[float]]:
    """由当前位移试算各段应力与轴力，不修改已提交历史。"""
    results: list[MaterialPointResult] = []
    forces: list[float] = []
    for index, segment in enumerate(model.segments):
        strain = (displacements[index + 1] - displacements[index]) / segment.length
        point = return_mapping(segment.material, committed[index], strain)
        results.append(point)
        forces.append(point.stress * segment.area)
    return results, forces


def _newton_step(
    model: BarModel,
    committed: list[MaterialState],
    prescribed: float,
    previous_free: list[float],
    tolerance: float,
    max_iterations: int,
) -> tuple[bool, int, list[float], list[MaterialPointResult], list[float], float]:
    """对单个加载步做 Newton 迭代；返回收敛标志与最终试算量。"""
    node_count = model.node_count
    free_count = node_count - 2
    displacements = [0.0] * node_count
    displacements[-1] = prescribed
    if free_count > 0:
        if previous_free:
            displacements[1:-1] = list(previous_free)
        else:
            for j in range(1, node_count - 1):
                fraction = j / (node_count - 1)
                displacements[j] = prescribed * fraction

    results, forces = _evaluate(model, committed, displacements)

    def residual_norm(force_list: list[float]) -> float:
        return max(
            (abs(force_list[j - 1] - force_list[j]) for j in range(1, node_count - 1)),
            default=0.0,
        )

    imbalance = residual_norm(forces)
    iterations = 0
    converged = False
    while iterations < max_iterations:
        force_scale = max(1.0, max(abs(f) for f in forces))
        if imbalance <= tolerance * force_scale:
            converged = True
            break
        residuals = [forces[j - 1] - forces[j] for j in range(1, node_count - 1)]
        matrix = [[0.0] * free_count for _ in range(free_count)]
        for j in range(1, node_count - 1):
            row = j - 1
            stiff_left = model.segments[j - 1].area * results[j - 1].tangent / model.segments[j - 1].length
            stiff_right = model.segments[j].area * results[j].tangent / model.segments[j].length
            matrix[row][row] = stiff_left + stiff_right
            if row > 0:
                matrix[row][row - 1] = -stiff_left
            if row < free_count - 1:
                matrix[row][row + 1] = -stiff_right
        delta = _solve_dense(matrix, [-r for r in residuals])

        # 回溯线搜索：屈服状态切换时全步可能发散，按残差下降收缩步长
        best = (imbalance, 0.0, results, forces)
        step_scale = 1.0
        for _ in range(8):
            trial = list(displacements)
            for j in range(free_count):
                trial[j + 1] += step_scale * delta[j]
            trial_results, trial_forces = _evaluate(model, committed, trial)
            trial_imbalance = residual_norm(trial_forces)
            if trial_imbalance < best[0]:
                best = (trial_imbalance, step_scale, trial_results, trial_forces)
                break
            step_scale *= 0.5
        if best[1] == 0.0:
            break
        for j in range(free_count):
            displacements[j + 1] += best[1] * delta[j]
        imbalance, results, forces = best[0], best[2], best[3]
        iterations += 1
    force_scale = max(1.0, max(abs(f) for f in forces))
    converged = converged or imbalance <= tolerance * force_scale
    return converged, iterations, displacements, results, forces, imbalance


def run_analysis(
    model: BarModel,
    displacements: list[float],
    tolerance: float = 1e-10,
    max_iterations: int = 50,
) -> AnalysisResult:
    """按顺序施加端部位移历程，逐步求解并提交材料历史。

    任一步达到迭代上限即停止：保留此前已收敛结果，失败步以
    converged=False 记录，结果字段为 None（绝不补零）。
    """
    if not math.isfinite(tolerance) or tolerance <= 0.0:
        raise SolverError(f"tolerance 必须为正有限数，得到 {tolerance!r}")
    if not isinstance(max_iterations, int) or max_iterations < 1:
        raise SolverError(f"max_iterations 必须为正整数，得到 {max_iterations!r}")
    for value in displacements:
        if not isinstance(value, (int, float)) or isinstance(value, bool) or not math.isfinite(value):
            raise SolverError(f"位移历程包含非法值 {value!r}")

    committed = [MaterialState() for _ in model.segments]
    previous_free: list[float] = []
    result = AnalysisResult()

    for step_index, prescribed in enumerate(displacements):
        converged, iterations, nodes, trial_results, forces, imbalance = _newton_step(
            model, committed, float(prescribed), previous_free, tolerance, max_iterations
        )
        if not converged:
            result.steps.append(
                StepResult(
                    step=step_index,
                    prescribed_displacement=float(prescribed),
                    converged=False,
                    iterations=iterations,
                    reaction_force=None,
                    axial_force=None,
                    node_displacements=None,
                    segments=None,
                    force_imbalance=imbalance,
                    compatibility_residual=None,
                )
            )
            result.failed_step = step_index
            break

        committed = [point.state for point in trial_results]
        previous_free = list(nodes[1:-1])
        compatibility = abs((nodes[-1] - nodes[0]) - sum(
            nodes[i + 1] - nodes[i] for i in range(len(model.segments))
        ))
        segment_results = tuple(
            SegmentResult(
                strain=(nodes[i + 1] - nodes[i]) / model.segments[i].length,
                stress=trial_results[i].stress,
                axial_force=forces[i],
                plastic_strain=trial_results[i].state.plastic_strain,
                accumulated_plastic_strain=trial_results[i].state.accumulated_plastic_strain,
                yielded=trial_results[i].yielded,
            )
            for i in range(len(model.segments))
        )
        result.steps.append(
            StepResult(
                step=step_index,
                prescribed_displacement=float(prescribed),
                converged=True,
                iterations=iterations,
                reaction_force=-forces[0],
                axial_force=forces[0],
                node_displacements=tuple(nodes),
                segments=segment_results,
                force_imbalance=imbalance,
                compatibility_residual=compatibility,
            )
        )
    return result
