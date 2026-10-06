"""CLI entry point: python -m plume_exposure CONFIG MET_CSV --out-dir DIR."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .engine import compute_contributions, summarize
from .io import ConfigError, load_config, load_met_periods


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="plume_exposure",
        description=(
            "Offline multi-source Gaussian plume exposure accounting. "
            "Screening estimates only; not a compliance determination."
        ),
    )
    parser.add_argument("config", help="JSON config: pollutant, sources, receptors, dispersion")
    parser.add_argument("met", help="CSV of met periods: start,end,wind_speed,wind_dir")
    parser.add_argument("--out-dir", default=".", help="output directory (default: cwd)")
    args = parser.parse_args(argv)

    try:
        config = load_config(args.config)
        periods = load_met_periods(args.met)
    except ConfigError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    contributions = compute_contributions(config, periods)
    summaries = summarize(config, periods, contributions)

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    csv_path = out_dir / "contributions.csv"
    with open(csv_path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "period_index",
                "period_start",
                "period_end",
                "source_id",
                "receptor_id",
                "concentration",
                "time_integral",
            ]
        )
        for c in contributions:
            period = periods[c.period_index]
            writer.writerow(
                [
                    c.period_index,
                    period.start.isoformat(),
                    period.end.isoformat(),
                    c.source_id,
                    c.receptor_id,
                    f"{c.concentration:.10g}",
                    f"{c.time_integral:.10g}",
                ]
            )

    json_path = out_dir / "summary.json"
    payload = {
        "pollutant": config.pollutant,
        "decay_rate": config.decay_rate,
        "period_count": len(periods),
        "note": (
            "Steady-state Gaussian plume screening estimates; "
            "not a regulatory compliance determination."
        ),
        "receptors": [
            {
                "receptor_id": s.receptor_id,
                "total_duration_s": s.total_duration,
                "average_concentration": s.average_concentration,
                "time_integral": s.time_integral,
                "peak_concentration": s.peak_concentration,
                "peak_period_index": s.peak_period_index,
                "peak_period_start": s.peak_period_start,
            }
            for s in summaries
        ],
    }
    json_path.write_text(
        json.dumps(payload, indent=2, ensure_ascii=False) + "\n", encoding="utf-8"
    )

    print(f"wrote {csv_path}")
    print(f"wrote {json_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
