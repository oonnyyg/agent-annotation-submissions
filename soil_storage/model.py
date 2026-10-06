"""土层参数、校验与静水平衡含水率剖面。"""

from __future__ import annotations

import math
from dataclasses import dataclass


def _finite(name: str, value: float) -> float:
    value = float(value)
    if not math.isfinite(value):
        raise ValueError(f"{name} 必须为有限数值，收到 {value!r}")
    return value


@dataclass(frozen=True)
class Layer:
    """一个土层的 van Genuchten 参数（自上而下顺序给出）。

    thickness: 土层厚度（cm），必须为正。
    theta_r:   残余体积含水率（0~1）。
    theta_s:   饱和体积含水率（0~1），须大于 theta_r。
    alpha:     尺度参数（1/cm），必须为正。
    n:         形状指数，必须大于 1；m 取 1 - 1/n。
    """

    thickness: float
    theta_r: float
    theta_s: float
    alpha: float
    n: float

    def __post_init__(self) -> None:
        thickness = _finite("thickness", self.thickness)
        theta_r = _finite("theta_r", self.theta_r)
        theta_s = _finite("theta_s", self.theta_s)
        alpha = _finite("alpha", self.alpha)
        n = _finite("n", self.n)

        if not (thickness > 0.0):
            raise ValueError(f"thickness 必须为正，收到 {thickness}")
        if not (0.0 <= theta_r < 1.0):
            raise ValueError("theta_r 必须满足 0 <= theta_r < 1")
        if not (0.0 < theta_s <= 1.0):
            raise ValueError("theta_s 必须满足 0 < theta_s <= 1")
        if not (theta_r < theta_s):
            raise ValueError("theta_r 必须小于 theta_s")
        if not (alpha > 0.0):
            raise ValueError(f"alpha 必须为正（1/cm），收到 {alpha}")
        if not (n > 1.0):
            raise ValueError(f"n 必须大于 1，收到 {n}")

        object.__setattr__(self, "thickness", thickness)
        object.__setattr__(self, "theta_r", theta_r)
        object.__setattr__(self, "theta_s", theta_s)
        object.__setattr__(self, "alpha", alpha)
        object.__setattr__(self, "n", n)

    @property
    def m(self) -> float:
        return 1.0 - 1.0 / self.n


@dataclass(frozen=True)
class SoilProfile:
    """自上而下的土层序列，记录各层的顶、底深度（cm）。"""

    layers: tuple[Layer, ...]
    boundaries: tuple[float, ...]

    @classmethod
    def from_layers(cls, layers: list[Layer]) -> "SoilProfile":
        if not layers:
            raise ValueError("至少需要一个土层")
        boundaries: list[float] = [0.0]
        top = 0.0
        for layer in layers:
            top += layer.thickness
            boundaries.append(top)
        return cls(tuple(layers), tuple(boundaries))

    @property
    def total_depth(self) -> float:
        return self.boundaries[-1]

    def layer_index_at(self, depth: float) -> int:
        """返回 depth 所在层的索引；层界按下层参数处理。"""
        if not (0.0 <= depth <= self.total_depth):
            raise ValueError(
                f"采样深度 {depth} 越界，允许范围 [0, {self.total_depth}] cm"
            )
        if depth >= self.total_depth:
            return len(self.layers) - 1
        for i in range(len(self.layers)):
            if depth < self.boundaries[i + 1]:
                return i
        raise AssertionError("unreachable")


def vg_water_content(layer: Layer, suction_head: float) -> float:
    """van Genuchten 持水曲线：由吸力水头 h>=0（cm）求体积含水率。

    theta(h) = theta_r + (theta_s - theta_r) / (1 + (alpha h)^n)^m
    """
    h = _finite("suction_head", suction_head)
    if h < 0.0:
        raise ValueError(f"吸力水头不能为负，收到 {h}")
    if h == 0.0:
        return layer.theta_s
    x = layer.alpha * h
    se = 1.0 / (1.0 + x**layer.n) ** layer.m
    return layer.theta_r + (layer.theta_s - layer.theta_r) * se


def water_content_at(
    profile: SoilProfile, depth: float, water_table_depth: float
) -> float:
    """静水平衡下 depth 处的体积含水率。

    地表深度为 0、向下为正；水位以上吸力水头 = 水位深度 - 测点深度，
    水位及以下（depth >= water_table_depth）饱和。
    """
    wtd = _finite("water_table_depth", water_table_depth)
    if wtd < 0.0:
        raise ValueError(f"地下水位深度不能为负，收到 {wtd}")
    idx = profile.layer_index_at(depth)
    layer = profile.layers[idx]
    if depth >= wtd:
        return layer.theta_s
    suction = wtd - depth
    return vg_water_content(layer, suction)
