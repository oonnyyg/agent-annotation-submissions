"""CSV 脉冲与 JSON 采集配置的读取与校验。"""

from __future__ import annotations

import csv
import json
from pathlib import Path

from .core import (
    AcquisitionConfig,
    ChannelConfig,
    DeadTimeModel,
    Pulse,
    ReplayError,
)


def load_config(path: str | Path) -> tuple[AcquisitionConfig, dict[str, ChannelConfig]]:
    raw = json.loads(Path(path).read_text(encoding="utf-8"))
    try:
        acq_raw = raw["acquisition"]
        acq = AcquisitionConfig(
            start_ns=int(acq_raw["start_ns"]),
            end_ns=int(acq_raw["end_ns"]),
            coincidence_window_ns=int(raw["coincidence_window_ns"]),
        )
        channels: dict[str, ChannelConfig] = {}
        for name, ch in raw["channels"].items():
            channels[name] = ChannelConfig(
                name=name,
                dead_time_ns=int(ch["dead_time_ns"]),
                model=DeadTimeModel(ch["model"]),
                time_offset_ns=int(ch.get("time_offset_ns", 0)),
            )
    except KeyError as exc:
        raise ReplayError(f"配置缺少字段: {exc}") from exc
    if len(channels) != 2:
        raise ReplayError("必须恰好配置两个通道")
    return acq, channels


def load_pulses(
    path: str | Path,
    channels: dict[str, ChannelConfig],
    acq: AcquisitionConfig,
) -> dict[str, list[Pulse]]:
    pulses: dict[str, list[Pulse]] = {name: [] for name in channels}
    seen_ids: set[str] = set()
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.DictReader(fh)
        required = {"pulse_id", "channel", "time_ns"}
        if reader.fieldnames is None or not required <= set(reader.fieldnames):
            raise ReplayError(f"脉冲 CSV 需要列: {sorted(required)}")
        for line_no, row in enumerate(reader, start=2):
            pulse_id = row["pulse_id"].strip()
            channel = row["channel"].strip()
            if not pulse_id:
                raise ReplayError(f"第 {line_no} 行: 空 pulse_id")
            if pulse_id in seen_ids:
                raise ReplayError(f"第 {line_no} 行: 重复编号 {pulse_id!r}")
            if channel not in channels:
                raise ReplayError(f"第 {line_no} 行: 未知通道 {channel!r}")
            try:
                time_ns = int(row["time_ns"])
            except ValueError:
                raise ReplayError(
                    f"第 {line_no} 行: time_ns 非整数 {row['time_ns']!r}"
                ) from None
            if not (acq.start_ns <= time_ns < acq.end_ns):
                raise ReplayError(
                    f"第 {line_no} 行: 脉冲 {pulse_id!r} 时间 {time_ns} "
                    f"在采集区间 [{acq.start_ns}, {acq.end_ns}) 之外"
                )
            seen_ids.add(pulse_id)
            pulses[channel].append(Pulse(pulse_id, channel, time_ns))
    return pulses
