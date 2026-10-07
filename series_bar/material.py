"""线性各向同性硬化材料的返回映射（应变驱动，闭合形式）。"""

from __future__ import annotations

from dataclasses import dataclass

from .model import Material


@dataclass(frozen=True)
class MaterialState:
    """单段材料历史：有符号塑性应变与单调不减的累计塑性应变。"""

    plastic_strain: float = 0.0
    accumulated_plastic_strain: float = 0.0


@dataclass(frozen=True)
class MaterialPointResult:
    """一次试算的结果，不修改传入的历史状态。"""

    stress: float
    tangent: float
    state: MaterialState
    yielded: bool
def return_mapping(
    material: Material, state: MaterialState, total_strain: float
) -> MaterialPointResult:
    """弹性预测 + 塑性修正，返回应力、一致切线模量与试算状态。

    屈服函数 f = |sigma| - (sy0 + H * alpha)，拉压对称。
    切线在塑性加载时为 E*H/(E+H)，弹性时为 E。
    """
    modulus = material.elastic_modulus
    trial_stress = modulus * (total_strain - state.plastic_strain)
    yield_limit = material.yield_strength + material.hardening_modulus * state.accumulated_plastic_strain
    trial_function = abs(trial_stress) - yield_limit

    if trial_function <= 0.0:
        return MaterialPointResult(trial_stress, modulus, state, yielded=False)

    direction = 1.0 if trial_stress > 0.0 else -1.0
    denominator = modulus + material.hardening_modulus
    delta_plastic = trial_function / denominator
    new_plastic_strain = state.plastic_strain + direction * delta_plastic
    new_accumulated = state.accumulated_plastic_strain + delta_plastic
    stress = trial_stress - modulus * direction * delta_plastic
    tangent = modulus * material.hardening_modulus / denominator
    new_state = MaterialState(new_plastic_strain, new_accumulated)
    return MaterialPointResult(stress, tangent, new_state, yielded=True)
