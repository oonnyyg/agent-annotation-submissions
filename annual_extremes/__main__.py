"""CLI: python -m annual_extremes observations.csv config.json out_series.csv out_stats.json"""

from __future__ import annotations

import argparse
import sys

from .bootstrap import bootstrap_return_levels
from .core import (
    build_annual_series,
    empirical_exceedance,
    fit_gumbel,
    return_level,
)
from .io import (
    read_config,
    read_observations,
    write_series_csv,
    write_stats_json,
)


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="annual_extremes",
        description="Gumbel annual-maxima extreme value analysis.",
    )
    parser.add_argument("observations", help="CSV with year,value columns")
    parser.add_argument("config", help="JSON config (return_periods, bootstrap)")
    parser.add_argument("series_out", help="output CSV for the annual series")
    parser.add_argument("stats_out", help="output JSON for statistics")
    args = parser.parse_args(argv)

    try:
        records = read_observations(args.observations)
        config = read_config(args.config)
        series = build_annual_series(records)
        if len(set(series.values)) == 1:
            raise ValueError("annual maxima are all equal; cannot fit Gumbel")
        fit = fit_gumbel(series.values)
        periods = config["return_periods"]
        levels = [(t, return_level(fit, t)) for t in periods]
        empirical = empirical_exceedance(series.years, series.values)
        intervals = bootstrap_return_levels(
            series.values, periods, config["resamples"], config["seed"]
        )
        write_series_csv(args.series_out, empirical)
        write_stats_json(
            args.stats_out,
            series,
            fit,
            levels,
            intervals,
            config["resamples"],
            config["seed"],
        )
    except (ValueError, OSError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
