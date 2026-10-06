"""实验执行：逐温度采样，生成 CSV 样本与 JSON 汇总。"""
from __future__ import annotations

import csv
import json

from .config import ExperimentConfig
from .exact import exact_observables
from .sampler import TemperatureResult, run_temperature

SAMPLED_KEYS = (
    "energy_per_spin",
    "abs_magnetization_per_spin",
    "specific_heat_per_spin",
    "susceptibility_per_spin",
)


def run_experiment(config: ExperimentConfig) -> tuple[dict, list[TemperatureResult]]:
    """运行全部温度点，返回 (汇总字典, 各温度采样结果)。"""
    results: list[TemperatureResult] = [run_temperature(config, t) for t in config.temperatures]
    exact_supported = config.size in (3, 4)
    entries = []
    for temp_config, result in zip(config.temperatures, results):
        sampled = {key: getattr(result, key) for key in SAMPLED_KEYS}
        entry = {
            "temperature": result.temperature,
            "thermalization_sweeps": temp_config.thermalization_sweeps,
            "sample_every": temp_config.sample_every,
            "n_samples": temp_config.n_samples,
            "sampled": sampled,
        }
        if exact_supported:
            exact = exact_observables(config.size, config.coupling, config.field, result.temperature)
            entry["exact"] = {key: getattr(exact, key) for key in SAMPLED_KEYS}
            entry["exact"]["n_states"] = exact.n_states
            entry["sampling_error"] = {
                key: sampled[key] - entry["exact"][key] for key in SAMPLED_KEYS
            }
        entries.append(entry)
    summary = {
        "size": config.size,
        "n_spins": config.n_spins,
        "coupling": config.coupling,
        "field": config.field,
        "seed": config.seed,
        "exact_enumeration": exact_supported,
        "temperatures": entries,
    }
    return summary, results


def write_samples_csv(path: str, config: ExperimentConfig, results: list[TemperatureResult]) -> None:
    with open(path, "w", encoding="utf-8", newline="") as handle:
        writer = csv.writer(handle)
        writer.writerow(["temperature", "sample_index", "sweep", "energy", "magnetization"])
        for result in results:
            for index, sample in enumerate(result.samples):
                writer.writerow(
                    [result.temperature, index, sample.sweep, sample.energy, sample.magnetization]
                )


def write_summary_json(path: str, summary: dict) -> None:
    with open(path, "w", encoding="utf-8") as handle:
        json.dump(summary, handle, ensure_ascii=False, indent=2)
        handle.write("\n")


__all__ = ["run_experiment", "write_samples_csv", "write_summary_json"]
