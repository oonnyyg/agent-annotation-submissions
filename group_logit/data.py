"""CSV and JSON config loading with strict validation."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass

from .errors import UserError


@dataclass
class Dataset:
    feature_names: list[str]
    features: list[list[float]]
    labels: list[int]
    groups: list[str]


@dataclass
class Config:
    label_column: str
    group_column: str
    fold_assignments: dict[str, int]
    lambdas: list[float]
    max_iterations: int
    tolerance: float


def _parse_float(raw: str, where: str) -> float:
    try:
        value = float(raw)
    except ValueError:
        raise UserError(f"{where}: value {raw!r} is not a number") from None
    if not math.isfinite(value):
        raise UserError(f"{where}: value {raw!r} is not finite")
    return value


def load_config(path: str) -> Config:
    try:
        with open(path, "r", encoding="utf-8") as fh:
            raw = json.load(fh)
    except OSError as exc:
        raise UserError(f"cannot read config {path}: {exc}") from None
    except json.JSONDecodeError as exc:
        raise UserError(f"config {path} is not valid JSON: {exc}") from None
    if not isinstance(raw, dict):
        raise UserError("config must be a JSON object")

    label_column = raw.get("label_column")
    group_column = raw.get("group_column")
    if not isinstance(label_column, str) or not label_column:
        raise UserError("config requires a non-empty string 'label_column'")
    if not isinstance(group_column, str) or not group_column:
        raise UserError("config requires a non-empty string 'group_column'")
    if label_column == group_column:
        raise UserError("label_column and group_column must differ")

    folds = raw.get("fold_assignments")
    if not isinstance(folds, dict) or not folds:
        raise UserError("config requires a non-empty object 'fold_assignments'")
    fold_assignments: dict[str, int] = {}
    for group, fold in folds.items():
        if not isinstance(group, str) or not group:
            raise UserError("fold_assignments keys must be non-empty group names")
        if isinstance(fold, bool) or not isinstance(fold, int) or fold < 0:
            raise UserError(f"fold for group {group!r} must be a non-negative integer")
        fold_assignments[group] = fold
    if len(set(fold_assignments.values())) < 2:
        raise UserError("fold_assignments must reference at least two distinct folds")

    lambdas = raw.get("lambdas")
    if not isinstance(lambdas, list) or not lambdas:
        raise UserError("config requires a non-empty array 'lambdas'")
    parsed_lambdas: list[float] = []
    for value in lambdas:
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise UserError(f"lambda {value!r} is not a number")
        if not math.isfinite(value) or value <= 0.0:
            raise UserError(f"lambda {value!r} must be a positive finite number")
        parsed_lambdas.append(float(value))
    if len(set(parsed_lambdas)) != len(parsed_lambdas):
        raise UserError("lambdas must not contain duplicates")

    max_iterations = raw.get("max_iterations", 200)
    if isinstance(max_iterations, bool) or not isinstance(max_iterations, int) or max_iterations < 1:
        raise UserError("max_iterations must be a positive integer")

    tolerance = raw.get("tolerance", 1e-8)
    if isinstance(tolerance, bool) or not isinstance(tolerance, (int, float)):
        raise UserError("tolerance must be a number")
    if not math.isfinite(tolerance) or tolerance <= 0.0:
        raise UserError("tolerance must be a positive finite number")

    return Config(
        label_column=label_column,
        group_column=group_column,
        fold_assignments=fold_assignments,
        lambdas=parsed_lambdas,
        max_iterations=max_iterations,
        tolerance=float(tolerance),
    )


def load_training_csv(path: str, config: Config) -> Dataset:
    try:
        with open(path, "r", encoding="utf-8", newline="") as fh:
            rows = list(csv.reader(fh))
    except OSError as exc:
        raise UserError(f"cannot read data {path}: {exc}") from None
    if not rows:
        raise UserError(f"data {path} is empty")
    header = rows[0]
    if len(header) != len(set(header)):
        raise UserError("CSV header contains duplicate column names")
    if config.label_column not in header:
        raise UserError(f"label column {config.label_column!r} not found in CSV header")
    if config.group_column not in header:
        raise UserError(f"group column {config.group_column!r} not found in CSV header")

    label_idx = header.index(config.label_column)
    group_idx = header.index(config.group_column)
    feature_names = [name for i, name in enumerate(header) if i not in (label_idx, group_idx)]
    if not feature_names:
        raise UserError("CSV must contain at least one feature column")
    feature_idx = [header.index(name) for name in feature_names]

    features: list[list[float]] = []
    labels: list[int] = []
    groups: list[str] = []
    for row_number, row in enumerate(rows[1:], start=2):
        if not row or all(cell.strip() == "" for cell in row):
            continue
        if len(row) != len(header):
            raise UserError(f"row {row_number}: expected {len(header)} columns, got {len(row)}")
        where = f"row {row_number}"
        raw_label = row[label_idx].strip()
        if raw_label not in ("0", "1"):
            raise UserError(f"{where}: label must be 0 or 1, got {raw_label!r}")
        group = row[group_idx].strip()
        if not group:
            raise UserError(f"{where}: group id is empty")
        if group not in config.fold_assignments:
            raise UserError(f"{where}: group {group!r} has no fold assignment in config")
        values = []
        for idx, name in zip(feature_idx, feature_names):
            cell = row[idx].strip()
            if cell == "":
                raise UserError(f"{where}: feature {name!r} is missing")
            values.append(_parse_float(cell, f"{where} feature {name!r}"))
        features.append(values)
        labels.append(int(raw_label))
        groups.append(group)

    if not features:
        raise UserError("CSV contains no data rows")
    return Dataset(feature_names=feature_names, features=features, labels=labels, groups=groups)
