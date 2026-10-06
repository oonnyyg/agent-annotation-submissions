"""Command line interface: run an experiment, write CSV samples and JSON summary."""

from __future__ import annotations

import argparse
import csv
import json
import sys
from pathlib import Path

from .experiment import load_experiment, run_experiment


def _write_samples_csv(path: Path, exp: dict, results: dict) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    with open(path, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["temperature", "sweep", "energy", "magnetization"])
        for t in exp["temperatures"]:
            for s in results[t]["samples"]:
                writer.writerow([t, s.sweep, s.energy, s.magnetization])


def _build_summary(exp: dict, results: dict) -> dict:
    spec = exp["spec"]
    entry = {
        "lattice": {
            "size": exp["size"],
            "n_sites": exp["size"] * exp["size"],
            "boundary": "periodic",
        },
        "coupling": exp["coupling"],
        "field": exp["field"],
        "boltzmann_constant": 1.0,
        "initial_state": "given" if exp["initial_spins"] is not None else "all_up",
        "schedule": {
            "equilibration_sweeps": spec.equilibration_sweeps,
            "sampling_sweeps": spec.sampling_sweeps,
            "sample_interval": spec.sample_interval,
            "seed": spec.seed,
        },
        "temperatures": [],
    }
    for t in exp["temperatures"]:
        row = {"temperature": t, "sampling": results[t]["summary"]}
        if results[t]["exact"] is not None:
            row["exact"] = results[t]["exact"]
        entry["temperatures"].append(row)
    return entry


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="ising-lab",
        description="2D Ising model Metropolis sampler with exact small-lattice check.",
    )
    parser.add_argument("config", help="path to the JSON experiment configuration")
    parser.add_argument(
        "--samples-csv", default="samples.csv", help="output CSV of raw samples"
    )
    parser.add_argument(
        "--summary-json", default="summary.json", help="output JSON summary"
    )
    args = parser.parse_args(argv)

    try:
        exp = load_experiment(args.config)
    except (OSError, json.JSONDecodeError, ValueError) as exc:
        print(f"configuration error: {exc}", file=sys.stderr)
        return 2

    results = run_experiment(exp)
    _write_samples_csv(Path(args.samples_csv), exp, results)
    summary = _build_summary(exp, results)
    Path(args.summary_json).parent.mkdir(parents=True, exist_ok=True)
    with open(args.summary_json, "w", encoding="utf-8") as fh:
        json.dump(summary, fh, indent=2)
        fh.write("\n")

    for row in summary["temperatures"]:
        s = row["sampling"]
        line = (
            f"T={row['temperature']:>8}  E/N={s['energy_per_spin']:+.6f}  "
            f"|M|/N={s['abs_magnetization_per_spin']:.6f}  "
            f"C/N={s['heat_capacity_per_spin']:.6f}  "
            f"chi/N={s['susceptibility_per_spin']:.6f}"
        )
        if "exact" in row:
            e = row["exact"]
            line += f"  | exact E/N={e['energy_per_spin']:+.6f}"
        print(line)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
