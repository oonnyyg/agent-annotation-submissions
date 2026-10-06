"""Orchestration: read inputs, validate, fit, bootstrap, write outputs."""

import csv
import json
import math

from .bootstrap import bootstrap_return_levels
from .gumbel import empirical_exceedance, fit_gumbel, return_level


def read_observations(path):
    """Read a CSV with year,value columns; duplicate years keep the max."""
    by_year = {}
    with open(path, "r", encoding="utf-8", newline="") as fh:
        for row in csv.DictReader(fh):
            year = int(row["year"])
            value = float(row["value"])
            if not math.isfinite(value):
                raise ValueError(f"non-finite value for year {year}")
            if year not in by_year or value > by_year[year]:
                by_year[year] = value
    return by_year


def validate(by_year):
    if len(by_year) < 3:
        raise ValueError("need at least three valid years")
    values = list(by_year.values())
    if all(v == values[0] for v in values):
        raise ValueError("annual maxima must not all be equal")


def missing_years(by_year):
    years = sorted(by_year)
    full = set(range(years[0], years[-1] + 1))
    return sorted(full - set(years))


def run_analysis(config):
    by_year = read_observations(config["input_csv"])
    validate(by_year)
    years = sorted(by_year)
    values = [by_year[y] for y in years]
    return_periods = [float(t) for t in config.get("return_periods", [])]
    for t in return_periods:
        if t <= 1.0:
            raise ValueError("return periods must be greater than one")

    fit = fit_gumbel(values)
    levels = {str(t): return_level(fit, t) for t in return_periods}
    empirical = empirical_exceedance(values)

    summary = {
        "n_years": len(years),
        "year_span": [years[0], years[-1]],
        "missing_years": missing_years(by_year),
        "gumbel": fit,
        "return_levels": levels,
    }

    boot_cfg = config.get("bootstrap")
    if boot_cfg:
        boot = bootstrap_return_levels(
            values, return_periods,
            int(boot_cfg.get("resamples", 1000)),
            int(boot_cfg.get("seed", 0)),
        )
        summary["bootstrap"] = {
            "resamples": int(boot_cfg.get("resamples", 1000)),
            "seed": int(boot_cfg.get("seed", 0)),
            "degenerate_resamples": boot["degenerate_resamples"],
            "intervals": {str(t): boot["intervals"][t] for t in return_periods},
            "note": "pointwise percentile intervals, not a simultaneous band",
        }

    out_csv = config.get("output_csv")
    if out_csv:
        _write_series_csv(out_csv, years, by_year, empirical)
    out_json = config.get("output_json")
    if out_json:
        with open(out_json, "w", encoding="utf-8") as fh:
            json.dump(summary, fh, indent=2, ensure_ascii=False)
    return {"summary": summary}


def _write_series_csv(path, years, by_year, empirical):
    emp_by_value = {}
    for row in empirical:
        emp_by_value.setdefault(row["value"], row)
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(["year", "value", "rank", "exceedance_probability"])
        for year in years:
            row = emp_by_value[by_year[year]]
            writer.writerow([year, by_year[year], row["rank"],
                             row["exceedance_probability"]])
