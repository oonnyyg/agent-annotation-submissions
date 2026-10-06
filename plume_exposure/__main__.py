"""Command line interface: python -m plume_exposure."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .config import ConfigError, load_met_periods, load_scenario
from .engine import compute_contributions, summarize


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="plume_exposure",
        description=(
            "Offline multi-source steady-state Gaussian plume exposure "
            "accounting (screening estimates only, not a compliance "
            "determination)."
        ),
    )
    parser.add_argument("scenario", help="JSON file with sources and receptors")
    parser.add_argument("met", help="CSV file with meteorological periods")
    parser.add_argument(
        "--contributions",
        default="contributions.csv",
        help="output CSV of per-source contributions (default: %(default)s)",
    )
    parser.add_argument(
        "--summary",
        default="summary.json",
        help="output JSON receptor summary (default: %(default)s)",
    )
    args = parser.parse_args(argv)

    try:
        scenario = load_scenario(args.scenario)
        periods = load_met_periods(args.met)
    except ConfigError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    except (OSError, json.JSONDecodeError) as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2

    contributions = compute_contributions(scenario, periods)
    summaries = summarize(scenario, periods, contributions)

    with open(args.contributions, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(
            [
                "period_index",
                "period_start",
                "period_end",
                "receptor_id",
                "source_id",
                "concentration",
            ]
        )
        for item in contributions:
            writer.writerow(
                [
                    item.period_index,
                    item.period_start,
                    item.period_end,
                    item.receptor_id,
                    item.source_id,
                    f"{item.concentration:.10g}",
                ]
            )

    summary_doc = {
        "note": (
            "Screening-level Gaussian plume estimates; not a regulatory "
            "compliance determination."
        ),
        "period_count": len(periods),
        "receptors": [
            {
                "receptor_id": item.receptor_id,
                "total_duration_s": item.total_duration_s,
                "time_integral": item.time_integral,
                "weighted_average": item.weighted_average,
                "peak_concentration": item.peak_concentration,
                "peak_period_index": item.peak_period_index,
                "peak_period_start": item.peak_period_start,
            }
            for item in summaries
        ],
    }
    Path(args.summary).write_text(
        json.dumps(summary_doc, indent=2) + "\n", encoding="utf-8"
    )
    print(
        f"wrote {len(contributions)} contribution rows to {args.contributions} "
        f"and {len(summaries)} receptor summaries to {args.summary}"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
