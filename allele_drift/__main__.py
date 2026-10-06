"""Command line interface: python -m allele_drift config.json --out-dir DIR"""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .core import ConfigError, run_scenario, validate_config


def load_scenarios(path):
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    if isinstance(raw, dict):
        raw = raw.get("scenarios", raw)
    if isinstance(raw, dict):
        raw = [raw]
    if not isinstance(raw, list) or not raw:
        raise ConfigError("input must be a scenario object, a non-empty "
                          "list, or {'scenarios': [...]}")
    return [validate_config(item, i) for i, item in enumerate(raw)]


def write_csv(result, path):
    scenario = result["scenario"]
    with open(path, "w", encoding="utf-8", newline="") as fh:
        writer = csv.writer(fh)
        writer.writerow([
            "generation", "count", "probability",
            "expected_frequency", "expected_heterozygosity_2f(1-f)",
            "first_hit_increment", "first_hit_cumulative",
            "untouched_probability",
        ])
        for row in result["generations"]:
            for count, prob in enumerate(row["distribution"]):
                writer.writerow([
                    row["generation"], count, repr(prob),
                    repr(row["expected_frequency"]),
                    repr(row["expected_heterozygosity"]),
                    repr(row["first_hit_increment"]),
                    repr(row["first_hit_cumulative"]),
                    repr(row["untouched_probability"]),
                ])


def summarize(result):
    scenario = result["scenario"]
    return {
        "name": scenario.name,
        "parameters": {
            "population_size": scenario.population_size,
            "initial_count": scenario.initial_count,
            "generations": scenario.generations,
            "selection_coefficient": scenario.selection_coefficient,
            "mutation_to_target": scenario.mutation_to_target,
            "mutation_from_target": scenario.mutation_from_target,
        },
        "max_probability_sum_error": result["max_probability_sum_error"],
        "generations": [
            {k: v for k, v in row.items() if k != "distribution"}
            for row in result["generations"]
        ],
    }


def main(argv=None):
    parser = argparse.ArgumentParser(
        prog="python -m allele_drift",
        description="Exact finite haploid allele drift calculator "
                    "(selection then bidirectional mutation).")
    parser.add_argument("config", help="JSON config: a scenario, a list, "
                                       "or {'scenarios': [...]}")
    parser.add_argument("--out-dir", default=".",
                        help="directory for CSV and summary output")
    args = parser.parse_args(argv)

    try:
        scenarios = load_scenarios(args.config)
    except (ConfigError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    out_dir = Path(args.out_dir)
    out_dir.mkdir(parents=True, exist_ok=True)

    summaries = []
    for scenario in scenarios:
        result = run_scenario(scenario)
        csv_path = out_dir / f"{scenario.name}.csv"
        write_csv(result, csv_path)
        summaries.append(summarize(result))

    summary_path = out_dir / "summary.json"
    with open(summary_path, "w", encoding="utf-8") as fh:
        json.dump({"scenarios": summaries}, fh, indent=2)
        fh.write("\n")
    print(f"wrote {len(summaries)} scenario(s) to {out_dir}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
