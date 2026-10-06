"""数据模型与输入解析（脉冲 CSV 与 JSON 采集配置）。"""

from __future__ import annotations

import csv
import json
from dataclasses import dataclass, field
from pathlib import Path


class ReplayError(ValueError):
    """输入数据或配置不合法。"""


@dataclass(frozen=True)
class ChannelConfig:
    name: str
    dead_time_ns: int
    extendable: bool
    time_offset_ns: int = 0


@dataclass(frozen=True)
class ReplayConfig:
    start_ns: int
    end_ns: int
    window_ns: int
    channels: tuple[ChannelConfig, ChannelConfig]

    @property
    def duration_ns(self) -> int:
        return self.end_ns - self.start_ns


@dataclass
class Pulse:
    channel: str
    event_id: str
    time_ns: int
    accepted: bool = False
    shifted_ns: int | None = None
    paired_with: str | None = None
    note: str = ""


def load_config(path: str | Path) -> ReplayConfig:
    raw = json.loads(Path(path).read_text(encoding="utf-8"))
    try:
        acq = raw["acquisition"]
        start_ns = int(acq["start_ns"])
        end_ns = int(acq["end_ns"])
        window_ns = int(raw["window_ns"])
        ch_raw = raw["channels"]
    except (KeyError, TypeError, ValueError) as exc:
        raise ReplayError(f"配置缺少必要字段或类型错误: {exc}") from exc
    if end_ns <= start_ns:
        raise ReplayError("采集区间 end_ns 必须大于 start_ns")
    if window_ns < 0:
        raise ReplayError("符合窗口 window_ns 不能为负")
    if not isinstance(ch_raw, dict) or len(ch_raw) != 2:
        raise ReplayError("channels 必须恰好包含两个通道")
    channels: list[ChannelConfig] = []
    for name, spec in ch_raw.items():
        dead_time_ns = int(spec.get("dead_time_ns", 0))
        if dead_time_ns < 0:
            raise ReplayError(f"通道 {name} 死时间不能为负")
        model = spec.get("model", "nonparalyzable")
        if model not in ("nonparalyzable", "extendable"):
            raise ReplayError(f"通道 {name} 未知死时间模型: {model}")
        channels.append(
            ChannelConfig(
                name=name,
                dead_time_ns=dead_time_ns,
                extendable=(model == "extendable"),
                time_offset_ns=int(spec.get("time_offset_ns", 0)),
            )
        )
    return ReplayConfig(
        start_ns=start_ns,
        end_ns=end_ns,
        window_ns=window_ns,
        channels=(channels[0], channels[1]),
    )


def load_pulses(path: str | Path, config: ReplayConfig) -> dict[str, list[Pulse]]:
    known = {c.name for c in config.channels}
    seen_ids: set[str] = set()
    pulses: dict[str, list[Pulse]] = {name: [] for name in known}
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.DictReader(fh)
        required = {"channel", "event_id", "time_ns"}
        if reader.fieldnames is None or not required <= set(reader.fieldnames):
            raise ReplayError("脉冲 CSV 需要表头: channel,event_id,time_ns")
        for lineno, row in enumerate(reader, start=2):
            channel = (row["channel"] or "").strip()
            event_id = (row["event_id"] or "").strip()
            if channel not in known:
                raise ReplayError(f"第 {lineno} 行: 未知通道 {channel!r}")
            if not event_id:
                raise ReplayError(f"第 {lineno} 行: event_id 为空")
            if event_id in seen_ids:
                raise ReplayError(f"第 {lineno} 行: 重复事件编号 {event_id!r}")
            try:
                time_ns = int(row["time_ns"])
            except (TypeError, ValueError) as exc:
                raise ReplayError(f"第 {lineno} 行: time_ns 不是整数") from exc
            if not (config.start_ns <= time_ns < config.end_ns):
                raise ReplayError(
                    f"第 {lineno} 行: 事件 {event_id!r} 时间 {time_ns} 在采集区间外"
                )
            seen_ids.add(event_id)
            pulses[channel].append(Pulse(channel, event_id, time_ns))
    for name in known:
        pulses[name].sort(key=lambda p: (p.time_ns, p.event_id))
    return pulses
