"""汇总回放结果并输出 CSV/JSON。"""

from __future__ import annotations

import csv
import json
from dataclasses import dataclass
from pathlib import Path

from .coincidence import Pair, apply_offsets, match_pairs
from .deadtime import busy_time_ns, simulate_channel
from .model import Pulse, ReplayConfig


@dataclass(frozen=True)
class ReplayResult:
    pulses: dict[str, list[Pulse]]
    pairs: list[Pair]
    summary: dict


def run_replay(pulses: dict[str, list[Pulse]], config: ReplayConfig) -> ReplayResult:
    summary_channels: dict[str, dict] = {}
    for channel in config.channels:
        ch_pulses = pulses[channel.name]
        intervals = simulate_channel(ch_pulses, channel)
        busy = busy_time_ns(intervals, config)
        live = config.duration_ns - busy
        accepted = sum(1 for p in ch_pulses if p.accepted)
        summary_channels[channel.name] = {
            "model": "extendable" if channel.extendable else "nonparalyzable",
            "dead_time_ns": channel.dead_time_ns,
            "time_offset_ns": channel.time_offset_ns,
            "input_count": len(ch_pulses),
            "accepted_count": accepted,
            "busy_time_ns": busy,
            "live_time_ns": live,
            "input_rate_per_ns": len(ch_pulses) / config.duration_ns,
            "accepted_rate_per_ns": (accepted / live) if live > 0 else None,
        }
    eligible = apply_offsets(pulses, config)
    pairs = match_pairs(eligible, config)
    summary = {
        "acquisition": {
            "start_ns": config.start_ns,
            "end_ns": config.end_ns,
            "duration_ns": config.duration_ns,
            "window_ns": config.window_ns,
        },
        "channels": summary_channels,
        "coincidence_count": len(pairs),
    }
    return ReplayResult(pulses=pulses, pairs=pairs, summary=summary)


def write_outputs(result: ReplayResult, out_dir: str | Path) -> None:
    out = Path(out_dir)
    out.mkdir(parents=True, exist_ok=True)
    with open(out / "pulses.csv", "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            ["channel", "event_id", "time_ns", "accepted",
             "shifted_ns", "paired_with", "note"]
        )
        for channel in sorted(result.pulses):
            for p in result.pulses[channel]:
                writer.writerow([
                    p.channel, p.event_id, p.time_ns,
                    "yes" if p.accepted else "no",
                    "" if p.shifted_ns is None else p.shifted_ns,
                    p.paired_with or "", p.note,
                ])
    with open(out / "pairs.csv", "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            ["pair_index", "event_a", "event_b",
             "time_a_ns", "time_b_ns", "diff_ns"]
        )
        for pair in result.pairs:
            writer.writerow([
                pair.index, pair.event_a, pair.event_b,
                pair.time_a_ns, pair.time_b_ns, pair.diff_ns,
            ])
    text = json.dumps(result.summary, ensure_ascii=False, indent=2)
    (out / "summary.json").write_text(text + "\n", encoding="utf-8")
