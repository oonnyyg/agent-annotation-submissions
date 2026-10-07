"""Loading and validation of sample CSV and target-margin JSON files."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass, field

from .errors import RakingError

KEY_SEPARATOR = "|"


@dataclass
class Sample:
    sample_id: str
    fields: dict[str, str]
    initial_weight: float


@dataclass
class Margin:
    name: str
    fields: tuple[str, ...]
    targets: dict[str, float]  # category key -> target total


@dataclass
class RakingConfig:
    tolerance: float = 1e-6
    max_iterations: int = 100
    margins: list[Margin] = field(default_factory=list)


def _no_duplicate_keys(pairs: list[tuple[str, object]]) -> dict[str, object]:
    seen: dict[str, object] = {}
    for key, value in pairs:
        if key in seen:
            raise RakingError(f"duplicate key in JSON: {key!r}")
        seen[key] = value
    return seen


def load_samples(path: str) -> tuple[list[Sample], list[str]]:
    """Read sample CSV. Returns (samples, categorical field names)."""
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.DictReader(fh)
        if reader.fieldnames is None:
            raise RakingError("sample CSV has no header row")
        columns = list(reader.fieldnames)
        if "id" not in columns:
            raise RakingError("sample CSV must contain an 'id' column")
        if "weight" not in columns:
            raise RakingError("sample CSV must contain a 'weight' column")
        field_names = [c for c in columns if c not in ("id", "weight")]
        if not field_names:
            raise RakingError("sample CSV must contain at least one categorical field")
        samples: list[Sample] = []
        seen_ids: set[str] = set()
        for lineno, row in enumerate(reader, start=2):
            sample_id = (row.get("id") or "").strip()
            if not sample_id:
                raise RakingError(f"line {lineno}: empty sample id")
            if sample_id in seen_ids:
                raise RakingError(f"line {lineno}: duplicate sample id {sample_id!r}")
            seen_ids.add(sample_id)
            try:
                weight = float(row["weight"])
            except (TypeError, ValueError):
                raise RakingError(
                    f"line {lineno}: invalid initial weight {row.get('weight')!r}"
                ) from None
            if not math.isfinite(weight) or weight <= 0.0:
                raise RakingError(
                    f"line {lineno}: initial weight must be positive and finite, "
                    f"got {weight!r}"
                )
            fields: dict[str, str] = {}
            for name in field_names:
                value = (row.get(name) or "").strip()
                if not value:
                    raise RakingError(
                        f"line {lineno}: empty value for field {name!r}"
                    )
                fields[name] = value
            samples.append(Sample(sample_id, fields, weight))
    if not samples:
        raise RakingError("sample CSV contains no rows")
    return samples, field_names


def category_key(fields: dict[str, str], names: tuple[str, ...]) -> str:
    return KEY_SEPARATOR.join(fields[n] for n in names)


def load_config(path: str, samples: list[Sample],
                available_fields: list[str]) -> RakingConfig:
    """Read target-margin JSON and validate it against the sample."""
    with open(path, encoding="utf-8") as fh:
        try:
            raw = json.load(fh, object_pairs_hook=_no_duplicate_keys)
        except json.JSONDecodeError as exc:
            raise RakingError(f"invalid JSON in targets file: {exc}") from None
    if not isinstance(raw, dict):
        raise RakingError("targets JSON must be an object")

    tolerance = raw.get("tolerance", 1e-6)
    max_iterations = raw.get("max_iterations", 100)
    if not isinstance(tolerance, (int, float)) or not tolerance > 0:
        raise RakingError("tolerance must be a positive number")
    if not isinstance(max_iterations, int) or max_iterations < 1:
        raise RakingError("max_iterations must be a positive integer")

    raw_margins = raw.get("margins")
    if not isinstance(raw_margins, list) or not raw_margins:
        raise RakingError("targets JSON must contain a non-empty 'margins' list")

    margins: list[Margin] = []
    margin_total: float | None = None
    seen_names: set[str] = set()
    for index, entry in enumerate(raw_margins):
        if not isinstance(entry, dict):
            raise RakingError(f"margin #{index + 1} must be an object")
        name = entry.get("name") or f"margin_{index + 1}"
        if name in seen_names:
            raise RakingError(f"duplicate margin name {name!r}")
        seen_names.add(name)
        fields = entry.get("fields")
        if (not isinstance(fields, list) or not fields
                or not all(isinstance(f, str) for f in fields)):
            raise RakingError(f"margin {name!r}: 'fields' must be a non-empty list")
        if len(set(fields)) != len(fields):
            raise RakingError(f"margin {name!r}: duplicate field names")
        for fname in fields:
            if fname not in available_fields:
                raise RakingError(
                    f"margin {name!r}: unknown field {fname!r} "
                    f"(available: {available_fields})"
                )
        raw_targets = entry.get("targets")
        if not isinstance(raw_targets, dict) or not raw_targets:
            raise RakingError(f"margin {name!r}: 'targets' must be a non-empty object")
        targets: dict[str, float] = {}
        for key, value in raw_targets.items():
            if not isinstance(value, (int, float)) or isinstance(value, bool):
                raise RakingError(
                    f"margin {name!r}: target for {key!r} must be a number"
                )
            value = float(value)
            if not math.isfinite(value) or value < 0.0:
                raise RakingError(
                    f"margin {name!r}: target for {key!r} must be non-negative "
                    f"and finite, got {value!r}"
                )
            parts = key.split(KEY_SEPARATOR)
            if len(parts) != len(fields):
                raise RakingError(
                    f"margin {name!r}: category key {key!r} does not match "
                    f"fields {fields}"
                )
            targets[key] = value
        total = math.fsum(targets.values())
        if total <= 0.0:
            raise RakingError(f"margin {name!r}: targets sum to zero")
        if margin_total is None:
            margin_total = total
        elif not math.isclose(total, margin_total, rel_tol=1e-9, abs_tol=1e-12):
            raise RakingError(
                f"margin {name!r}: total {total} differs from common "
                f"margin total {margin_total}"
            )
        margins.append(Margin(name, tuple(fields), targets))

    # Every category combination present in the sample must have a target.
    for margin in margins:
        for sample in samples:
            key = category_key(sample.fields, margin.fields)
            if key not in margin.targets:
                raise RakingError(
                    f"margin {margin.name!r}: sample category {key!r} "
                    f"has no target"
                )
    return RakingConfig(float(tolerance), max_iterations, margins)
