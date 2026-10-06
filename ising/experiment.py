"""Experiment driver: load JSON config, run all temperatures, aggregate."""

from __future__ import annotations

import json
from pathlib import Path

from .exact import exact_observables
from .mc import RunSpec, run_temperature
from .model import IsingConfig
from .stats import summarize_samples


def load_experiment(path: str | Path) -> dict:
    """Load and validate the JSON experiment configuration."""
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    required = ("size", "coupling", "temperatures", "seed")
    for key in required:
        if key not in raw:
            raise ValueError(f"missing required key: {key}")
    temps = raw["temperatures"]
    if not isinstance(temps, list) or not temps:
        raise ValueError("temperatures must be a non-empty list")
    if len(set(temps)) != len(temps):
        raise ValueError("duplicate temperatures are not allowed")
    size = raw["size"]
    spins = raw.get("initial_spins")
    for t in temps:
        IsingConfig(
            size=size,
            coupling=raw["coupling"],
            temperature=t,
            field=raw.get("field", 0.0),
            initial_spins=tuple(spins) if spins is not None else None,
        )
    spec = RunSpec(
        equilibration_sweeps=raw.get("equilibration_sweeps", 1000),
        sampling_sweeps=raw.get("sampling_sweeps", 10000),
        sample_interval=raw.get("sample_interval", 10),
        seed=raw["seed"],
    )
    return {
        "size": size,
        "coupling": raw["coupling"],
        "field": raw.get("field", 0.0),
        "temperatures": temps,
        "initial_spins": spins,
        "spec": spec,
    }


def run_experiment(exp: dict) -> dict:
    """Run every temperature independently and collect samples + summaries."""
    spec: RunSpec = exp["spec"]
    temps = exp["temperatures"]
    # Rank in the sorted list seeds each temperature, so reordering the
    # input list cannot change any temperature's random stream.
    ranks = {t: i for i, t in enumerate(sorted(temps))}
    n_sites = exp["size"] * exp["size"]
    results = {}
    for t in temps:
        config = IsingConfig(
            size=exp["size"],
            coupling=exp["coupling"],
            temperature=t,
            field=exp["field"],
            initial_spins=tuple(exp["initial_spins"])
            if exp["initial_spins"] is not None
            else None,
        )
        samples = run_temperature(config, spec, ranks[t])
        summary = summarize_samples(samples, n_sites, t)
        exact = None
        if exp["size"] in (3, 4):
            exact = exact_observables(config)
            summary["sampling_error"] = {
                key: summary[key] - exact[key]
                for key in (
                    "energy_per_spin",
                    "abs_magnetization_per_spin",
                    "heat_capacity_per_spin",
                    "susceptibility_per_spin",
                )
            }
        results[t] = {"samples": samples, "summary": summary, "exact": exact}
    return results
