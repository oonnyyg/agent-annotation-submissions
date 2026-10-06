"""JSON 实验配置的解析与校验。"""

from __future__ import annotations

import json
import math
from dataclasses import dataclass, field


class ConfigError(ValueError):
    """配置不合法时抛出。"""


@dataclass(frozen=True)
class TemperatureConfig:
    """单个温度点的采样参数。"""

    temperature: float
    thermalization_sweeps: int
    sample_every: int
    n_samples: int


@dataclass(frozen=True)
class ExperimentConfig:
    """整个实验的配置。"""

    size: int
    coupling: float
    field: float
    seed: int
    initial_spins: tuple[int, ...]  # 全 1 表示全正初态
    temperatures: tuple[TemperatureConfig, ...] = field(default_factory=tuple)

    @property
    def n_spins(self) -> int:
        return self.size * self.size


def _require_finite_positive(name: str, value: object) -> float:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ConfigError(f"{name} 必须是数字，得到 {value!r}")
    result = float(value)
    if not math.isfinite(result) or result <= 0.0:
        raise ConfigError(f"{name} 必须是有限正数，得到 {value!r}")
    return result


def _require_finite(name: str, value: object) -> float:
    if not isinstance(value, (int, float)) or isinstance(value, bool):
        raise ConfigError(f"{name} 必须是数字，得到 {value!r}")
    result = float(value)
    if not math.isfinite(result):
        raise ConfigError(f"{name} 必须有限，得到 {value!r}")
    return result


def _require_positive_int(name: str, value: object) -> int:
    if not isinstance(value, int) or isinstance(value, bool) or value <= 0:
        raise ConfigError(f"{name} 必须是正整数，得到 {value!r}")
    return value


def _parse_temperatures(raw: object) -> tuple[TemperatureConfig, ...]:
    if not isinstance(raw, list) or not raw:
        raise ConfigError("temperatures 必须是非空列表")
    configs: list[TemperatureConfig] = []
    seen: set[float] = set()
    defaults = {
        "thermalization_sweeps": 100,
        "sample_every": 1,
        "n_samples": 100,
    }
    for entry in raw:
        if isinstance(entry, (int, float)) and not isinstance(entry, bool):
            entry = {"temperature": entry}
        if not isinstance(entry, dict):
            raise ConfigError(f"温度条目必须是数字或对象，得到 {entry!r}")
        merged = {**defaults, **entry}
        temp = _require_finite_positive("temperature", merged["temperature"])
        if temp in seen:
            raise ConfigError(f"温度重复：{temp}")
        seen.add(temp)
        configs.append(
            TemperatureConfig(
                temperature=temp,
                thermalization_sweeps=_require_positive_int(
                    "thermalization_sweeps", merged["thermalization_sweeps"]
                ),
                sample_every=_require_positive_int("sample_every", merged["sample_every"]),
                n_samples=_require_positive_int("n_samples", merged["n_samples"]),
            )
        )
    return tuple(configs)


def _parse_initial_spins(raw: object, n_spins: int) -> tuple[int, ...]:
    if raw is None or raw == "all_up":
        return (1,) * n_spins
    if not isinstance(raw, list) or len(raw) != n_spins:
        raise ConfigError(f"initial_spins 必须是长度为 {n_spins} 的列表或 \"all_up\"")
    spins = []
    for value in raw:
        if value not in (1, -1):
            raise ConfigError(f"自旋只能取 +1 或 -1，得到 {value!r}")
        spins.append(int(value))
    return tuple(spins)


def parse_config(data: object) -> ExperimentConfig:
    if not isinstance(data, dict):
        raise ConfigError("配置必须是 JSON 对象")
    size = data.get("size")
    if not isinstance(size, int) or isinstance(size, bool) or not 3 <= size <= 64:
        raise ConfigError(f"size 必须是 3 到 64 的整数，得到 {size!r}")
    coupling = _require_finite_positive("coupling", data.get("coupling", 1.0))
    field_value = _require_finite("field", data.get("field", 0.0))
    seed = data.get("seed", 0)
    if not isinstance(seed, int) or isinstance(seed, bool):
        raise ConfigError(f"seed 必须是整数，得到 {seed!r}")
    initial_spins = _parse_initial_spins(data.get("initial_spins"), size * size)
    temperatures = _parse_temperatures(data.get("temperatures"))
    return ExperimentConfig(
        size=size,
        coupling=coupling,
        field=field_value,
        seed=seed,
        initial_spins=initial_spins,
        temperatures=temperatures,
    )


def load_config(path: str) -> ExperimentConfig:
    with open(path, "r", encoding="utf-8") as handle:
        data = json.load(handle)
    return parse_config(data)
