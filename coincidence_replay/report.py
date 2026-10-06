"""汇总统计与 CSV/JSON 输出。"""
from __future__ import annotations

import csv
import json
from pathlib import Path

from .core import (
    AcquisitionConfig,
    ChannelResult,
    CoincidencePair,
    UnpairedPulse,
)


def _rate(count: int, duration_ns: int) -> float | None:
    """计数率（次/秒）；分母为零时返回 None。"""
    if duration_ns <= 0:
        return None
    return count * 1e9 / duration_ns


def build_summary(
    acq: AcquisitionConfig,
    results: dict[str, ChannelResult],
    pairs: list[CoincidencePair],
    unpaired: list[UnpairedPulse],
) -> dict:
    channels_out = {}
    for name, res in results.items():
        live_ns = res.live_time_ns(acq)
        channels_out[name] = {
            "model": res.config.model.value,
            "dead_time_ns": res.config.dead_time_ns,
            "time_offset_ns": res.config.time_offset_ns,
            "input_count": res.input_count,
            "accepted_count": res.accepted_count,
            "rejected_count": res.input_count - res.accepted_count,
            "busy_time_ns": res.busy_time_ns(acq),
            "live_time_ns": live_ns,
            "input_rate_hz": _rate(res.input_count, acq.duration_ns),
            "accepted_rate_hz": _rate(res.accepted_count, live_ns),
        }
    return {
        "acquisition": {
            "start_ns": acq.start_ns,
            "end_ns": acq.end_ns,
            "duration_ns": acq.duration_ns,
            "coincidence_window_ns": acq.coincidence_window_ns,
        },
        "channels": channels_out,
        "coincidences": {
            "pair_count": len(pairs),
            "unpaired_count": len(unpaired),
        },
    }


def write_pulse_status_csv(results: dict[str, ChannelResult], path: str | Path) -> None:
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["channel", "pulse_id", "time_ns", "status", "reason"])
        for name in sorted(results):
            for rec in results[name].records:
                writer.writerow(
                    [name, rec.pulse.pulse_id, rec.pulse.time_ns,
                     rec.status.value, rec.reason]
                )


def write_pairs_csv(
    pairs: list[CoincidencePair],
    unpaired: list[UnpairedPulse],
    offset_a_ns: int,
    offset_b_ns: int,
    path: str | Path,
) -> None:
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow([
            "kind", "channel_a_pulse", "channel_b_pulse",
            "time_a_ns", "time_b_ns", "delta_ns", "reason",
        ])
        for pair in pairs:
            ta = pair.pulse_a.time_ns + offset_a_ns
            tb = pair.pulse_b.time_ns + offset_b_ns
            writer.writerow([
                "pair", pair.pulse_a.pulse_id, pair.pulse_b.pulse_id,
                ta, tb, pair.delta_ns, "",
            ])
        for unp in unpaired:
            writer.writerow([
                "unpaired", unp.pulse.pulse_id, "",
                unp.shifted_time_ns, "", "", unp.reason,
            ])


def write_summary_json(summary: dict, path: str | Path) -> None:
    Path(path).write_text(
        json.dumps(summary, ensure_ascii=False, indent=2) + "\n",
        encoding="utf-8",
    )
