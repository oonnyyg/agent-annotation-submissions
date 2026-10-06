"""双通道探测器回放工具：死时间分析与符合事件配对。"""

from .core import (
    AcquisitionConfig,
    ChannelConfig,
    ChannelResult,
    CoincidencePair,
    Pulse,
    PulseStatus,
    ReplayError,
    UnpairedPulse,
    find_coincidences,
    replay_channel,
)

__all__ = [
    "AcquisitionConfig",
    "ChannelConfig",
    "ChannelResult",
    "CoincidencePair",
    "Pulse",
    "PulseStatus",
    "ReplayError",
    "UnpairedPulse",
    "find_coincidences",
    "replay_channel",
]
