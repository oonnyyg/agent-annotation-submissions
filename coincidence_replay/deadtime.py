"""死时间模拟与忙/活时间统计。"""

from __future__ import annotations

from .model import ChannelConfig, Pulse, ReplayConfig


def simulate_channel(
    pulses: list[Pulse], channel: ChannelConfig
) -> list[tuple[int, int]]:
    """按时间顺序回放，标记接收状态，返回死区区间列表 [start, end)。"""
    dead_intervals: list[tuple[int, int]] = []
    dead_until: int | None = None
    for pulse in pulses:
        busy = dead_until is not None and pulse.time_ns < dead_until
        if busy:
            pulse.accepted = False
            pulse.note = "rejected_dead_time"
            if channel.extendable:
                dead_until = max(dead_until, pulse.time_ns + channel.dead_time_ns)
                dead_intervals.append((pulse.time_ns, dead_until))
        else:
            pulse.accepted = True
            dead_until = pulse.time_ns + channel.dead_time_ns
            dead_intervals.append((pulse.time_ns, dead_until))
    return dead_intervals


def busy_time_ns(
    intervals: list[tuple[int, int]], config: ReplayConfig
) -> int:
    """死区并集与采集区间 [start, end) 的交集总长度。"""
    total = 0
    cur_start: int | None = None
    cur_end = 0
    for start, end in sorted(intervals):
        start = max(start, config.start_ns)
        end = min(end, config.end_ns)
        if end <= start:
            continue
        if cur_start is None:
            cur_start, cur_end = start, end
        elif start <= cur_end:
            cur_end = max(cur_end, end)
        else:
            total += cur_end - cur_start
            cur_start, cur_end = start, end
    if cur_start is not None:
        total += cur_end - cur_start
    return total
