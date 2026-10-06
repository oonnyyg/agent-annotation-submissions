"""CLI: python -m gage_study measurements.csv config.json --outdir out"""

from __future__ import annotations

import argparse
import csv
import json
import os
import sys

from .data import InputError, load_config, load_observations, validate_structure
from .model import analyze


def _fmt(value):
    if value is None:
        return ""
    if isinstance(value, float):
        return repr(value)
    return value


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="gage_study",
        description="Crossed gage R&R study (balanced two-factor random effects).",
    )
    parser.add_argument("csv", help="measurement CSV file")
    parser.add_argument("config", help="JSON config file")
    parser.add_argument("--outdir", default=".", help="output directory")
    args = parser.parse_args(argv)

    try:
        config = load_config(args.config)
        observations = load_observations(args.csv, config["columns"])
        parts, operators, n_replicates = validate_structure(observations)
        result = analyze(observations, parts, operators, n_replicates)
    except InputError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    os.makedirs(args.outdir, exist_ok=True)

    anova_path = os.path.join(args.outdir, "anova.csv")
    with open(anova_path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(["source", "df", "ss", "ms"])
        for row in result.anova:
            writer.writerow([row.source, row.df, _fmt(row.ss), _fmt(row.ms)])

    var_path = os.path.join(args.outdir, "variance_components.csv")
    with open(var_path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow(["component", "variance", "stddev",
                         "percent_variance", "percent_stddev"])
        for name, estimate in result.variance_components.items():
            s = result.summary[name]
            writer.writerow([name, _fmt(estimate), _fmt(s["stddev"]),
                             _fmt(s["percent_variance"]), _fmt(s["percent_stddev"])])
        for name in ("reproducibility", "gage", "total"):
            s = result.summary[name]
            writer.writerow([name, _fmt(s["variance"]), _fmt(s["stddev"]),
                             _fmt(s["percent_variance"]), _fmt(s["percent_stddev"])])

    diag = {
        "parts": result.parts,
        "operators": result.operators,
        "replicates_per_cell": result.n_replicates,
        "cell_means": [
            {"part": p, "operator": op, "mean": result.cell_means[(p, op)]}
            for p in result.parts for op in result.operators
        ],
        "residuals": [
            {"part": p, "operator": op, "replicate": rep, "residual": res}
            for p, op, rep, res in sorted(result.residuals)
        ],
        "ss_check": result.ss_check,
        "warnings": result.warnings,
    }
    diag_path = os.path.join(args.outdir, "diagnostics.json")
    with open(diag_path, "w", encoding="utf-8") as fh:
        json.dump(diag, fh, indent=2, ensure_ascii=False)
        fh.write("\n")

    print(f"wrote {anova_path}, {var_path}, {diag_path}")
    for warning in result.warnings:
        print(f"warning: {warning}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
