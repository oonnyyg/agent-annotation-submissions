"""核心回放逻辑：死时间模型、活时间统计与符合配对。

时间统一为整数纳秒。采集区间为左闭右开 [start_ns, end_ns)。
"""

from __future__ import annotations

from dataclasses import dataclass, field
from enum import Enum


class ReplayError(ValueError):
    """输入数据或配置非法时抛出。"""


class DeadTimeModel(str, Enum):
    NONPARALYZABLE = "nonparalyzable"
    EXTENDABLE = "extendable"


@dataclass(frozen=True)
class ChannelConfig:
    name: str
    dead_time_ns: int
    model: DeadTimeModel
    time_offset_ns: int = 0

    def __post_init__(self) -> None:
        if self.dead_time_ns < 0:
            raise ReplayError(f"通道 {self.name} 死时间不能为负: {self.dead_time_ns}")


@dataclass(frozen=True)
class AcquisitionConfig:
    start_ns: int
    end_ns: int
    coincidence_window_ns: int

    def __post_init__(self) -> None:
        if self.end_ns <= self.start_ns:
            raise ReplayError("采集区间 end_ns 必须大于 start_ns")
        if self.coincidence_window_ns < 0:
            raise ReplayError("符合窗口不能为负")

    @property
    def duration_ns(self) -> int:
        return self.end_ns - self.start_ns


@dataclass(frozen=True)
class Pulse:
    pulse_id: str
    channel: str
    time_ns: int


class PulseStatus(str, Enum):
    ACCEPTED = "accepted"
    REJECTED = "rejected"


@dataclass(frozen=True)
class PulseRecord:
    pulse: Pulse
    status: PulseStatus
    reason: str


@dataclass
class ChannelResult:
    config: ChannelConfig
    records: list[PulseRecord] = field(default_factory=list)
    busy_intervals: list[tuple[int, int]] = field(default_factory=list)

    @property
    def input_count(self) -> int:
        return len(self.records)

    @property
    def accepted_count(self) -> int:
        return sum(1 for r in self.records if r.status is PulseStatus.ACCEPTED)

    @property
    def accepted_pulses(self) -> list[Pulse]:
        return [r.pulse for r in self.records if r.status is PulseStatus.ACCEPTED]

    def busy_time_ns(self, acq: AcquisitionConfig) -> int:
        """死区并集与采集区间的交集长度，不能简单累加每次死时间。"""
        total = 0
        for start, end in self.busy_intervals:
            lo = max(start, acq.start_ns)
            hi = min(end, acq.end_ns)
            if hi > lo:
                total += hi - lo
        return total

    def live_time_ns(self, acq: AcquisitionConfig) -> int:
        return acq.duration_ns - self.busy_time_ns(acq)


def replay_channel(
    pulses: list[Pulse], config: ChannelConfig, acq: AcquisitionConfig
) -> ChannelResult:
    """按 (时间, 编号字典序) 排序后逐脉冲回放死时间模型。

    死区为半开区间 [触发时刻, 触发时刻+死时间)，恰好到结束时刻可接收。
    """
    ordered = sorted(pulses, key=lambda p: (p.time_ns, p.pulse_id))
    result = ChannelResult(config=config)
    busy_end: int | None = None
    for pulse in ordered:
        free = busy_end is None or pulse.time_ns >= busy_end
        if free:
            result.records.append(PulseRecord(pulse, PulseStatus.ACCEPTED, ""))
            new_end = pulse.time_ns + config.dead_time_ns
            result.busy_intervals.append((pulse.time_ns, new_end))
            busy_end = new_end
        else:
            result.records.append(
                PulseRecord(pulse, PulseStatus.REJECTED, "dead_time")
            )
            if config.model is DeadTimeModel.EXTENDABLE:
                new_end = pulse.time_ns + config.dead_time_ns
                if new_end > busy_end:
                    busy_end = new_end
                    # 与上一区间重叠，直接合并为并集
                    start, _ = result.busy_intervals[-1]
                    result.busy_intervals[-1] = (start, busy_end)
    return result


@dataclass(frozen=True)
class CoincidencePair:
    pulse_a: Pulse
    pulse_b: Pulse
    delta_ns: int  # b 校时后时间 - a 校时后时间


@dataclass(frozen=True)
class UnpairedPulse:
    pulse: Pulse
    shifted_time_ns: int
    reason: str


def find_coincidences(
    accepted_a: list[Pulse],
    accepted_b: list[Pulse],
    offset_a_ns: int,
    offset_b_ns: int,
    window_ns: int,
    acq: AcquisitionConfig,
) -> tuple[list[CoincidencePair], list[UnpairedPulse]]:
    """贪心依次比较两路最早未用脉冲，时间差绝对值不超窗口即配对。

    这不是最近邻搜索：每个脉冲只用一次，失配时丢弃较早者。
    校时后落在采集区间外的脉冲不参与符合。
    """

    def shifted(pulses: list[Pulse], offset: int) -> list[tuple[Pulse, int]]:
        out = []
        for p in pulses:
            t = p.time_ns + offset
            if acq.start_ns <= t < acq.end_ns:
                out.append((p, t))
        return out

    list_a = shifted(accepted_a, offset_a_ns)
    list_b = shifted(accepted_b, offset_b_ns)
    pairs: list[CoincidencePair] = []
    unpaired: list[UnpairedPulse] = []
    i = j = 0
    while i < len(list_a) and j < len(list_b):
        pa, ta = list_a[i]
        pb, tb = list_b[j]
        if abs(ta - tb) <= window_ns:
            pairs.append(CoincidencePair(pa, pb, tb - ta))
            i += 1
            j += 1
        elif ta < tb:
            unpaired.append(UnpairedPulse(pa, ta, "no_partner_in_window"))
            i += 1
        else:
            unpaired.append(UnpairedPulse(pb, tb, "no_partner_in_window"))
            j += 1
    for p, t in list_a[i:]:
        unpaired.append(UnpairedPulse(p, t, "channel_exhausted"))
    for p, t in list_b[j:]:
        unpaired.append(UnpairedPulse(p, t, "channel_exhausted"))
    return pairs, unpaired
