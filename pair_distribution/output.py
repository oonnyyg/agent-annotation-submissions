"""CSV and JSON summary writers."""

from __future__ import annotations

import csv
import json

from .config import Config
from .rdf import PairResult
from .trajectory import Frame


def write_csv(path: str, config: Config, results: list[PairResult]) -> None:
    n_frames = len(results[0].histograms) if results and results[0].histograms else 0
    max_frames = max((len(r.histograms) for r in results), default=0)
    n_frames = max(n_frames, max_frames)
    frame_cols = [f"frame_{i}_count" for i in range(n_frames)]
    header = [
        "pair", "bin", "r_lower", "r_upper", "r_mid",
        "raw_count", "g_r", "coordination",
    ] + frame_cols
    dr = config.dr
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(header)
        for result in results:
            totals = result.total_counts
            for k in range(config.n_bins):
                r_lo = k * dr
                row = [
                    result.label,
                    k,
                    f"{r_lo:.10g}",
                    f"{r_lo + dr:.10g}",
                    f"{r_lo + 0.5 * dr:.10g}",
                    totals[k],
                    "" if result.empty else f"{result.g_avg[k]:.10g}",
                    f"{result.cn_avg[k]:.10g}",
                ]
                for hist in result.histograms:
                    row.append(hist.counts[k])
                for _ in range(n_frames - len(result.histograms)):
                    row.append("")
                writer.writerow(row)


def write_summary(
    path: str,
    config: Config,
    frames: list[Frame],
    results: list[PairResult],
    r_max_limit: float,
) -> None:
    species = sorted({t for f in frames for t, _ in f.particles.values()})
    summary = {
        "n_frames": len(frames),
        "n_particles": len(frames[0].particles),
        "species": species,
        "box_lengths": {
            "min": [min(f.box[i] for f in frames) for i in range(3)],
            "max": [max(f.box[i] for f in frames) for i in range(3)],
        },
        "r_max": config.r_max,
        "r_max_allowed": r_max_limit,
        "n_bins": config.n_bins,
        "dr": config.dr,
        "normalization": {
            "shell_volume": "exact 4/3*pi*(r_hi^3 - r_lo^3) per frame",
            "density": "per-frame target count / per-frame volume; "
                        "same-species uses (N-1)/V",
            "average": "unweighted mean over frames (frame-order independent)",
        },
        "pairs": [
            {
                "pair": r.label,
                "empty": r.empty,
                "reason": r.reason,
                "total_raw_count": sum(r.total_counts),
                "mean_n_centers": (
                    sum(h.n_centers for h in r.histograms) / len(r.histograms)
                    if r.histograms else 0
                ),
            }
            for r in results
        ],
    }
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(summary, handle, indent=2)
        handle.write("\n")
