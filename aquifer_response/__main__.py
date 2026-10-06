
"""Command line interface: python -m aquifer_response <config.json>.

Writes a CSV time series per observation point and a JSON analysis
report. If the computation fails, existing output files are left
untouched.
"""

from __future__ import annotations

import argparse
import csv
import json
import sys

from .analysis import rank_candidates
from .io_config import ConfigError, load_config
from .model import ModelError
from .well_function import WellFunctionError


def _build_csv_rows(config, contributions):
    header = ["point", "time_s", "total_drawdown_m"] + [
        f"drawdown_{well.name}_m" for well in config.model.wells
    ]
    rows = []
    for point in config.points:
        per_well = contributions[point.name]
        for i, t in enumerate(config.times):
            values = [per_well[w.name][i] for w in config.model.wells]
            rows.append(
                [point.name, f"{t:.12g}", f"{sum(values):.12g}"]
                + [f"{v:.12g}" for v in values]
            )
    return header, rows


def run(config_path: str, csv_path: str, report_path: str) -> int:
    try:
        config = load_config(config_path)
        contributions = config.model.contributions(config.points, config.times)
        header, rows = _build_csv_rows(config, contributions)

        report = {
            "aquifer": {
                "transmissivity_m2_per_s": config.model.transmissivity,
                "storativity": config.model.storativity,
            },
            "well_function": {
                "tolerance": config.model.tol,
                "max_terms": config.model.max_terms,
                "status": "converged",
            },
            "points": [p.name for p in config.points],
            "wells": [w.name for w in config.model.wells],
            "times_s": config.times,
            "contributions_m": {
                point: {well: vals for well, vals in per_well.items()}
                for point, per_well in contributions.items()
            },
            "total_drawdown_m": {
                point.name: [
                    sum(vals)
                    for vals in zip(*contributions[point.name].values())
                ]
                if config.model.wells
                else [0.0] * len(config.times)
                for point in config.points
            },
        }
        if config.candidates:
            if not config.observed:
                raise ConfigError(
                    "candidates require observed drawdown data to compare against"
                )
            ranking = rank_candidates(config)
            report["candidate_ranking"] = ranking
            report["best_candidate"] = ranking[0]["name"]

        # Only write outputs after every computation above has succeeded,
        # so a failure never overwrites existing reports.
        with open(csv_path, "w", encoding="utf-8", newline="") as fh:
            writer = csv.writer(fh)
            writer.writerow(header)
            writer.writerows(rows)
        with open(report_path, "w", encoding="utf-8") as fh:
            json.dump(report, fh, indent=2)
            fh.write("\n")
    except (ConfigError, ModelError, WellFunctionError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m aquifer_response",
        description="Multi-well pumping response in an ideal confined aquifer.",
    )
    parser.add_argument("config", help="input JSON configuration file")
    parser.add_argument(
        "--csv", default="drawdown_timeseries.csv",
        help="output CSV path (default: drawdown_timeseries.csv)",
    )
    parser.add_argument(
        "--report", default="report.json",
        help="output JSON report path (default: report.json)",
    )
    args = parser.parse_args(argv)
    return run(args.config, args.csv, args.report)


if __name__ == "__main__":
    sys.exit(main())
