"""Independent current schemes sharing the same coil geometry."""

from __future__ import annotations

import json
import math
from dataclasses import dataclass
from pathlib import Path

from .model import Coil


class ScenarioDataError(ValueError):
    """Raised when a scenario JSON file fails validation."""


@dataclass(frozen=True)
class Scenario:
    name: str
    currents: tuple[float, ...]  # one signed current per coil (amperes)


def baseline_scenario(coils: list[Coil]) -> Scenario:
    return Scenario("baseline", tuple(c.current for c in coils))


def load_scenarios(path: str | Path, coils: list[Coil]) -> list[Scenario]:
    """Load alternative current schemes; geometry always comes from `coils`.

    JSON format:
        {"scenarios": [{"name": "reversed_a",
                        "currents": {"coil_a": -12.5}}]}
    Coils not listed keep their baseline current.
    """
    data = json.loads(Path(path).read_text(encoding="utf-8"))
    if not isinstance(data, dict) or not isinstance(data.get("scenarios"), list):
        raise ScenarioDataError("top level must be an object with a 'scenarios' list")
    by_name = {c.name: c for c in coils}
    scenarios = []
    for i, raw in enumerate(data["scenarios"]):
        where = f"scenarios[{i}]"
        if not isinstance(raw, dict):
            raise ScenarioDataError(f"{where}: must be an object")
        name = raw.get("name", f"scenario_{i}")
        if not isinstance(name, str) or not name:
            raise ScenarioDataError(f"{where}: name must be a non-empty string")
        overrides = raw.get("currents", {})
        if not isinstance(overrides, dict):
            raise ScenarioDataError(f"{where}: currents must be an object of coil name to amperes")
        currents = list(c.current for c in coils)
        for coil_name, value in overrides.items():
            if coil_name not in by_name:
                raise ScenarioDataError(f"{where}: unknown coil {coil_name!r}")
            if (
                not isinstance(value, (int, float))
                or isinstance(value, bool)
                or not math.isfinite(value)
            ):
                raise ScenarioDataError(f"{where}: current for {coil_name!r} must be finite")
            idx = [j for j, c in enumerate(coils) if c.name == coil_name][0]
            currents[idx] = float(value)
        scenarios.append(Scenario(name, tuple(currents)))
    return scenarios
