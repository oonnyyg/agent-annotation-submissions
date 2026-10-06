"""CSV and JSON output helpers."""

from __future__ import annotations

import csv
from pathlib import Path

from .model import ScenarioResult


def write_scenario_csv(path: Path, result: ScenarioResult) -> None:
    n = result.scenario.population_size
    with path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(
            [
                "generation",
                "count",
                "probability",
                "expected_frequency",
                "expected_heterozygosity",
                "probability_sum",
                "prob_untouched_boundary",
                "boundary_hit_increment",
                "boundary_hit_cumulative",
            ]
        )
        for row in result.rows:
            for count in range(n + 1):
                writer.writerow(
                    [
                        row.generation,
                        count,
                        repr(row.probabilities[count]),
                        repr(row.expected_frequency),
                        repr(row.expected_heterozygosity),
                        repr(row.probability_sum),
                        repr(row.prob_untouched_boundary),
                        repr(row.boundary_hit_increment),
                        repr(row.boundary_hit_cumulative),
                    ]
                )


def build_summary(result: ScenarioResult, csv_path: Path) -> dict:
    sc = result.scenario
    last = result.rows[-1]
    return {
        "name": result.name,
        "parameters": {
            "population_size": sc.population_size,
            "initial_count": sc.initial_count,
            "generations": sc.generations,
            "selection_coefficient": sc.selection_coefficient,
            "mutation_to_target": sc.mutation_to_target,
            "mutation_to_ordinary": sc.mutation_to_ordinary,
        },
        "csv": str(csv_path),
        "final_expected_frequency": last.expected_frequency,
        "final_expected_heterozygosity": last.expected_heterozygosity,
        "final_prob_untouched_boundary": last.prob_untouched_boundary,
        "boundary_hit_cumulative": last.boundary_hit_cumulative,
        "boundary_hit_increments": [
            {"generation": r.generation, "increment": r.boundary_hit_increment}
            for r in result.rows
        ],
        "max_probability_sum_error": result.max_sum_error,
    }
