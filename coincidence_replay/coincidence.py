"""校时与两路符合配对。"""

from __future__ import annotations

from dataclasses import dataclass

from .model import Pulse, ReplayConfig


@dataclass(frozen=True)
class Pair:
    index: int
    event_a: str
    event_b: str
    time_a_ns: int
    time_b_ns: int
    diff_ns: int


def apply_offsets(
    pulses: dict[str, list[Pulse]], config: ReplayConfig
) -> dict[str, list[Pulse]]:
    """接收脉冲加各通道校时偏移，区间外的不参与符合。"""
    result: dict[str, list[Pulse]] = {}
    for channel in config.channels:
        eligible: list[Pulse] = []
        for pulse in pulses[channel.name]:
            if not pulse.accepted:
                continue
            shifted = pulse.time_ns + channel.time_offset_ns
            pulse.shifted_ns = shifted
            if config.start_ns <= shifted < config.end_ns:
                eligible.append(pulse)
            else:
                pulse.note = "out_of_range_after_offset"
        eligible.sort(key=lambda p: (p.shifted_ns, p.event_id))
        result[channel.name] = eligible
    return result


def match_pairs(
    eligible: dict[str, list[Pulse]], config: ReplayConfig
) -> list[Pair]:
    """依次比较两路最早未用脉冲，贪心配对（非最近邻搜索）。"""
    name_a, name_b = config.channels[0].name, config.channels[1].name
    list_a = eligible[name_a]
    list_b = eligible[name_b]
    pairs: list[Pair] = []
    idx_a = idx_b = 0
    while idx_a < len(list_a) and idx_b < len(list_b):
        pa, pb = list_a[idx_a], list_b[idx_b]
        diff = pa.shifted_ns - pb.shifted_ns
        if abs(diff) <= config.window_ns:
            pa.paired_with = pb.event_id
            pb.paired_with = pa.event_id
            pa.note = pb.note = "paired"
            pairs.append(
                Pair(len(pairs) + 1, pa.event_id, pb.event_id,
                     pa.shifted_ns, pb.shifted_ns, diff)
            )
            idx_a += 1
            idx_b += 1
        elif diff < 0:
            pa.note = "unmatched_no_partner"
            idx_a += 1
        else:
            pb.note = "unmatched_no_partner"
            idx_b += 1
    for leftover in list_a[idx_a:] + list_b[idx_b:]:
        leftover.note = "unmatched_no_partner"
    return pairs
