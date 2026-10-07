from __future__ import annotations

import csv
from dataclasses import dataclass
from pathlib import Path

from .errors import ObservationError


@dataclass(frozen=True)
class Observation:
    sequence_id: str
    position: int
    value: int


def load_observations(path: str | Path) -> dict[str, list[Observation]]:
    """Load the CSV and return ordered observations per sequence.

    Any invalid row rejects the whole file.
    """
    rows: list[Observation] = []
    with open(path, newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        required = {"sequence_id", "position", "value"}
        if reader.fieldnames is None or not required.issubset(reader.fieldnames):
            raise ObservationError(
                "CSV must contain columns: sequence_id, position, value")
        for line, row in enumerate(reader, start=2):
            seq = (row.get("sequence_id") or "").strip()
            if not seq:
                raise ObservationError(f"line {line}: empty sequence_id")
            try:
                position = int((row.get("position") or "").strip())
            except ValueError:
                raise ObservationError(
                    f"line {line}: position must be an integer") from None
            if position < 1:
                raise ObservationError(f"line {line}: position must be >= 1")
            raw = (row.get("value") or "").strip()
            if raw not in ("0", "1"):
                raise ObservationError(f"line {line}: value must be 0 or 1")
            rows.append(Observation(seq, position, int(raw)))

    grouped: dict[str, list[Observation]] = {}
    seen: dict[str, set[int]] = {}
    for obs in rows:
        positions = seen.setdefault(obs.sequence_id, set())
        if obs.position in positions:
            raise ObservationError(
                f"duplicate position {obs.position} in sequence "
                f"'{obs.sequence_id}'")
        positions.add(obs.position)
        grouped.setdefault(obs.sequence_id, []).append(obs)
    for obs_list in grouped.values():
        obs_list.sort(key=lambda o: o.position)
    return grouped
