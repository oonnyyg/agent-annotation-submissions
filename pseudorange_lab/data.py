"""Loading and validation of teaching observation data.

CSV columns (one header row):
    epoch, sat_id, x_m, y_m, z_m, pseudorange_m, sigma_m

JSON initial values:
    {"epochs": {"<epoch>": {"position_m": [x, y, z], "clock_m": c}}}
"""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass, field

from .errors import DataError

REQUIRED_COLUMNS = (
    "epoch",
    "sat_id",
    "x_m",
    "y_m",
    "z_m",
    "pseudorange_m",
    "sigma_m",
)


@dataclass(frozen=True)
class Observation:
    sat_id: str
    position_m: tuple[float, float, float]
    pseudorange_m: float
    sigma_m: float


@dataclass
class Epoch:
    name: str
    observations: list[Observation] = field(default_factory=list)
    initial_position_m: tuple[float, float, float] | None = None
    initial_clock_m: float | None = None


def _finite(value: float, what: str) -> float:
    if not math.isfinite(value):
        raise DataError(f"{what} must be finite, got {value!r}")
    return value


def load_observations_csv(path: str) -> dict[str, Epoch]:
    """Read the observation CSV and group rows by epoch."""
    epochs: dict[str, Epoch] = {}
    with open(path, newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        missing = [c for c in REQUIRED_COLUMNS if c not in (reader.fieldnames or [])]
        if missing:
            raise DataError(f"CSV missing columns: {', '.join(missing)}")
        for lineno, row in enumerate(reader, start=2):
            where = f"CSV line {lineno}"
            try:
                epoch_name = row["epoch"].strip()
                sat_id = row["sat_id"].strip()
                if not epoch_name or not sat_id:
                    raise DataError(f"{where}: empty epoch or sat_id")
                pos = tuple(
                    _finite(float(row[axis]), f"{where}: {axis}")
                    for axis in ("x_m", "y_m", "z_m")
                )
                pr = _finite(float(row["pseudorange_m"]), f"{where}: pseudorange_m")
                sigma = _finite(float(row["sigma_m"]), f"{where}: sigma_m")
            except ValueError as exc:
                raise DataError(f"{where}: {exc}") from exc
            if pr <= 0.0:
                raise DataError(f"{where}: pseudorange_m must be positive")
            if sigma <= 0.0:
                raise DataError(f"{where}: sigma_m must be positive")
            epoch = epochs.setdefault(epoch_name, Epoch(name=epoch_name))
            if any(o.sat_id == sat_id for o in epoch.observations):
                raise DataError(f"{where}: duplicate sat_id {sat_id!r} in epoch {epoch_name!r}")
            epoch.observations.append(
                Observation(sat_id=sat_id, position_m=pos, pseudorange_m=pr, sigma_m=sigma)
            )
    for epoch in epochs.values():
        if len(epoch.observations) < 4:
            raise DataError(
                f"epoch {epoch.name!r}: need at least 4 satellites, "
                f"got {len(epoch.observations)}"
            )
    return epochs


def load_initial_json(path: str, epochs: dict[str, Epoch]) -> None:
    """Attach per-epoch initial position and clock (meters) to epochs."""
    with open(path, encoding="utf-8") as handle:
        try:
            payload = json.load(handle)
        except json.JSONDecodeError as exc:
            raise DataError(f"invalid JSON: {exc}") from exc
    try:
        table = payload["epochs"]
    except (TypeError, KeyError) as exc:
        raise DataError("JSON must contain an 'epochs' object") from exc
    for name, epoch in epochs.items():
        entry = table.get(name)
        if entry is None:
            raise DataError(f"JSON has no initial values for epoch {name!r}")
        try:
            pos = tuple(float(v) for v in entry["position_m"])
            clock = float(entry["clock_m"])
        except (TypeError, KeyError, ValueError) as exc:
            raise DataError(f"epoch {name!r}: bad initial values: {exc}") from exc
        if len(pos) != 3 or not all(math.isfinite(v) for v in pos):
            raise DataError(f"epoch {name!r}: position_m must be 3 finite numbers")
        _finite(clock, f"epoch {name!r}: clock_m")
        epoch.initial_position_m = (pos[0], pos[1], pos[2])
        epoch.initial_clock_m = clock
