"""分段杆模型与材料参数定义，以及 JSON 输入的加载与校验。"""

from __future__ import annotations

import json
import math
from dataclasses import dataclass, field
from pathlib import Path


class ModelError(ValueError):
    """输入模型或参数非法时抛出。"""


def _finite_number(value, name: str) -> float:
    if isinstance(value, bool) or not isinstance(value, (int, float)):
        raise ModelError(f"{name} 必须是数值，得到 {value!r}")
    result = float(value)
    if not math.isfinite(result):
        raise ModelError(f"{name} 必须是有限数值，得到 {value!r}")
    return result


def _positive(value, name: str) -> float:
    result = _finite_number(value, name)
    if result <= 0.0:
        raise ModelError(f"{name} 必须为正数，得到 {result!r}")
    return result


@dataclass(frozen=True)
class Material:
    """线性各向同性硬化弹塑性材料。"""

    elastic_modulus: float
    yield_strength: float
    hardening_modulus: float

    @classmethod
    def from_dict(cls, data: dict, context: str) -> "Material":
        if not isinstance(data, dict):
            raise ModelError(f"{context} 必须是对象")
        try:
            modulus = _positive(data["elastic_modulus"], f"{context}.elastic_modulus")
            yield_strength = _positive(data["yield_strength"], f"{context}.yield_strength")
            hardening = _positive(data["hardening_modulus"], f"{context}.hardening_modulus")
        except KeyError as exc:
            raise ModelError(f"{context} 缺少字段 {exc.args[0]}") from None
        return cls(modulus, yield_strength, hardening)


@dataclass(frozen=True)
class Segment:
    """一个杆段：长度、截面积与材料。"""

    length: float
    area: float
    material: Material


@dataclass(frozen=True)
class BarModel:
    """左端固定、右端施加位移的串联杆。"""

    segments: tuple[Segment, ...]
    name: str = "series-bar"

    @property
    def node_count(self) -> int:
        return len(self.segments) + 1

    @property
    def total_length(self) -> float:
        return sum(seg.length for seg in self.segments)


def load_model(path: str | Path) -> BarModel:
    """从 JSON 文件加载模型并做严格校验。"""
    try:
        raw = Path(path).read_text(encoding="utf-8-sig")
    except OSError as exc:
        raise ModelError(f"无法读取模型文件 {path}: {exc}") from exc
    try:
        data = json.loads(raw)
    except json.JSONDecodeError as exc:
        raise ModelError(f"模型文件不是合法 JSON: {exc}") from exc
    if not isinstance(data, dict):
        raise ModelError("模型 JSON 顶层必须是对象")

    segments_data = data.get("segments")
    if not isinstance(segments_data, list) or not segments_data:
        raise ModelError("segments 必须是非空数组")

    default_material = None
    if "material" in data:
        default_material = Material.from_dict(data["material"], "material")

    segments: list[Segment] = []
    for index, item in enumerate(segments_data):
        context = f"segments[{index}]"
        if not isinstance(item, dict):
            raise ModelError(f"{context} 必须是对象")
        length = _positive(item.get("length"), f"{context}.length")
        area = _positive(item.get("area"), f"{context}.area")
        if "material" in item:
            material = Material.from_dict(item["material"], f"{context}.material")
        elif default_material is not None:
            material = default_material
        else:
            raise ModelError(f"{context} 缺少 material，且顶层未提供默认 material")
        segments.append(Segment(length, area, material))

    name = data.get("name", "series-bar")
    if not isinstance(name, str):
        raise ModelError("name 必须是字符串")
    return BarModel(tuple(segments), name=name)
