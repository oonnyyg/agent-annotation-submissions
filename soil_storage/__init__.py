"""静水平衡下的分层土壤储水核算工具。

仅计算静水平衡状态：给定地下水位深度，按各层自身的 van Genuchten
持水曲线得到体积含水率剖面，并沿深度积分得到储水厚度；亦可根据目标
总储水量反求地下水位。本工具不模拟入渗或非饱和流动过程。

长度单位统一为厘米（cm）。
"""

from .model import (
    Layer,
    SoilProfile,
    vg_water_content,
    water_content_at,
)
from .storage import StorageResult, compute_storage
from .invert import InvertResult, invert_water_table
from .batch import BatchRow, run_batch

__all__ = [
    "Layer",
    "SoilProfile",
    "vg_water_content",
    "water_content_at",
    "StorageResult",
    "compute_storage",
    "InvertResult",
    "invert_water_table",
    "BatchRow",
    "run_batch",
]
