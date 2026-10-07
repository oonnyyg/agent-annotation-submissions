"""Core raking logic: input validation, iterative proportional fitting, diagnostics."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass, field
from pathlib import Path


class RakingError(Exception):
    """Raised for invalid inputs (bad CSV/JSON, inconsistent targets, ...)."""


@dataclass(frozen=True)
class Margin:
    """One target margin over a single field or a combination of fields.

    ``targets`` maps the category key (a tuple of field values, one element
    per entry in ``fields``) to the non-negative target total weight.
    """

    name: str
    fields: tuple[str, ...]
    targets: dict[tuple[str, ...], float]


@dataclass
class RoundDiagnostic:
    """Per-round convergence diagnostic."""

    round_index: int
    max_residual: float
    worst_margin: str
    worst_category: tuple[str, ...]


@dataclass
class RakingResult:
    """Outcome of a raking run.

    ``converged`` is True only when every margin satisfied the tolerance at
    the end of some round. Diagnostics are always kept, even on failure.
    """

    converged: bool
    weights: list[float]
    rounds: list[RoundDiagnostic]
    achieved: dict[str, dict[tuple[str, ...], float]]
    failure_reason: str | None = None
    effective_sample_size: float | None = None


def load_samples(path: str | Path, id_field: str, weight_field: str) -> tuple[list[str], list[dict[str, str]], list[float]]:
    """Read sample rows from CSV.

    Returns (ids, rows, initial_weights). Rejects duplicate ids, missing
    fields and non-positive or non-finite initial weights.
    """
    ids: list[str] = []
    rows: list[dict[str, str]] = []
    weights: list[float] = []
    seen: set[str] = set()
    with open(path, newline="", encoding="utf-8-sig") as fh:
        reader = csv.DictReader(fh)
        if reader.fieldnames is None:
            raise RakingError("CSV 文件缺少表头")
        for required in (id_field, weight_field):
            if required not in reader.fieldnames:
                raise RakingError(f"CSV 缺少必需列: {required}")
        for line_no, row in enumerate(reader, start=2):
            sample_id = (row.get(id_field) or "").strip()
            if not sample_id:
                raise RakingError(f"第 {line_no} 行样本标识为空")
            if sample_id in seen:
                raise RakingError(f"样本标识重复: {sample_id}")
            seen.add(sample_id)
            raw = (row.get(weight_field) or "").strip()
            try:
                weight = float(raw)
            except ValueError as exc:
                raise RakingError(f"第 {line_no} 行初始权重无法解析: {raw!r}") from exc
            if not math.isfinite(weight) or weight <= 0.0:
                raise RakingError(f"第 {line_no} 行初始权重必须为正且有限: {raw!r}")
            ids.append(sample_id)
            rows.append({k: (v or "") for k, v in row.items()})
            weights.append(weight)
    if not ids:
        raise RakingError("CSV 中没有样本行")
    return ids, rows, weights


def load_targets(path: str | Path) -> list[Margin]:
    """Read target margins from JSON.

    Expected shape::

        {"margins": [
            {"name": "age", "fields": ["age"],
             "targets": {"young": 120.0, "old": 80.0}},
            {"name": "sex_x_region", "fields": ["sex", "region"],
             "targets": {"F||north": 50.0, ...}}
        ]}

    Multi-field category keys join the field values with "||" in the order
    of ``fields``. Targets must be non-negative and finite, every margin must
    have the same positive total, duplicate categories are rejected, and no
    margin may be silently dropped.
    """
    with open(path, encoding="utf-8") as fh:
        try:
            data = json.load(fh)
        except json.JSONDecodeError as exc:
            raise RakingError(f"目标边际 JSON 解析失败: {exc}") from exc
    if not isinstance(data, dict) or not isinstance(data.get("margins"), list) or not data["margins"]:
        raise RakingError("目标文件必须包含非空的 margins 列表")
    margins: list[Margin] = []
    expected_total: float | None = None
    seen_names: set[str] = set()
    for index, item in enumerate(data["margins"]):
        if not isinstance(item, dict):
            raise RakingError(f"第 {index} 个边际不是对象")
        name = item.get("name")
        fields = item.get("fields")
        targets = item.get("targets")
        if not isinstance(name, str) or not name:
            raise RakingError(f"第 {index} 个边际缺少有效 name")
        if name in seen_names:
            raise RakingError(f"边际名称重复: {name}")
        seen_names.add(name)
        if not isinstance(fields, list) or not fields or not all(isinstance(f, str) and f for f in fields):
            raise RakingError(f"边际 {name} 的 fields 必须是非空字符串列表")
        if len(set(fields)) != len(fields):
            raise RakingError(f"边际 {name} 的 fields 存在重复字段")
        if not isinstance(targets, dict) or not targets:
            raise RakingError(f"边际 {name} 的 targets 必须是非空对象")
        parsed: dict[tuple[str, ...], float] = {}
        for key, value in targets.items():
            if not isinstance(value, (int, float)) or isinstance(value, bool):
                raise RakingError(f"边际 {name} 类别 {key!r} 的目标不是数值")
            target = float(value)
            if not math.isfinite(target) or target < 0.0:
                raise RakingError(f"边际 {name} 类别 {key!r} 的目标必须非负且有限")
            category = tuple(key.split("||"))
            if len(category) != len(fields):
                raise RakingError(
                    f"边际 {name} 类别键 {key!r} 的维度与 fields 数量不一致"
                )
            if category in parsed:
                raise RakingError(f"边际 {name} 存在重复类别键: {key!r}")
            parsed[category] = target
        total = math.fsum(parsed.values())
        if not math.isfinite(total) or total <= 0.0:
            raise RakingError(f"边际 {name} 的目标总量必须为正")
        if expected_total is None:
            expected_total = total
        elif not math.isclose(total, expected_total, rel_tol=1e-9, abs_tol=1e-9):
            raise RakingError(
                f"边际 {name} 的目标总量 {total} 与首个边际总量 {expected_total} 不一致"
            )
        margins.append(Margin(name=name, fields=tuple(fields), targets=parsed))
    return margins


def _category_key(row: dict[str, str], fields: tuple[str, ...]) -> tuple[str, ...]:
    return tuple(row[f] for f in fields)


def _margin_totals(
    keys: list[tuple[str, ...]], weights: list[float]
) -> dict[tuple[str, ...], float]:
    totals: dict[tuple[str, ...], float] = {}
    for key, weight in zip(keys, weights):
        totals[key] = totals.get(key, 0.0) + weight
    return totals


def rake(
    rows: list[dict[str, str]],
    initial_weights: list[float],
    margins: list[Margin],
    *,
    tolerance: float = 1e-6,
    max_rounds: int = 100,
) -> RakingResult:
    """Iterative proportional fitting over the given margins, in config order.

    Each round visits every margin once; within a margin, each category's
    sample weights are multiplied by target/current. A zero target zeroes the
    category's weights; a positive target with no sample support or with all
    current weights already zero is reported as unreachable (no new samples
    are fabricated). After each round every margin is re-evaluated with
    residual |achieved - target| / max(target, 1); all margins must be within
    ``tolerance`` for convergence. Exceeding ``max_rounds`` is reported as
    non-convergence. Overflow and non-finite weights abort with diagnostics.
    """
    if tolerance <= 0 or not math.isfinite(tolerance):
        raise RakingError("容差必须为正且有限")
    if max_rounds < 1:
        raise RakingError("最大轮数必须至少为 1")

    weights = list(initial_weights)
    keys_per_margin: list[list[tuple[str, ...]]] = []
    for margin in margins:
        for field_name in margin.fields:
            if any(field_name not in row for row in rows):
                raise RakingError(f"样本数据缺少边际 {margin.name} 所需字段: {field_name}")
        keys = [_category_key(row, margin.fields) for row in rows]
        sample_categories = set(keys)
        missing = sample_categories - set(margin.targets)
        if missing:
            pretty = sorted("||".join(cat) for cat in missing)
            raise RakingError(
                f"边际 {margin.name} 的目标缺少样本中出现的类别: {pretty}"
            )
        keys_per_margin.append(keys)

    rounds: list[RoundDiagnostic] = []
    failure: str | None = None
    converged = False

    for round_index in range(1, max_rounds + 1):
        for margin, keys in zip(margins, keys_per_margin):
            totals = _margin_totals(keys, weights)
            for category, target in margin.targets.items():
                current = totals.get(category, 0.0)
                if target == 0.0:
                    if current > 0.0:
                        for i, key in enumerate(keys):
                            if key == category:
                                weights[i] = 0.0
                    continue
                if current == 0.0:
                    failure = (
                        f"边际 {margin.name} 类别 {'||'.join(category)} 目标为正 "
                        f"但样本支持为空或当前权重已全为零，目标不可达"
                    )
                    break
                ratio = target / current
                for i, key in enumerate(keys):
                    if key == category:
                        weights[i] *= ratio
            if failure:
                break
            for weight in weights:
                if not math.isfinite(weight):
                    failure = f"第 {round_index} 轮出现非有限权重（溢出）"
                    break
            if failure:
                break
        if failure:
            break
        worst = RoundDiagnostic(round_index, -1.0, "", ())
        for margin, keys in zip(margins, keys_per_margin):
            totals = _margin_totals(keys, weights)
            for category, target in margin.targets.items():
                achieved = totals.get(category, 0.0)
                residual = abs(achieved - target) / max(target, 1.0)
                if residual > worst.max_residual:
                    worst = RoundDiagnostic(round_index, residual, margin.name, category)
        rounds.append(worst)
        if worst.max_residual <= tolerance:
            converged = True
            break

    achieved: dict[str, dict[tuple[str, ...], float]] = {}
    for margin, keys in zip(margins, keys_per_margin):
        totals = _margin_totals(keys, weights)
        achieved[margin.name] = {
            category: totals.get(category, 0.0) for category in margin.targets
        }

    if not converged and failure is None:
        failure = f"达到最大轮数 {max_rounds} 仍未满足容差 {tolerance}"

    ess: float | None = None
    if converged:
        total = math.fsum(weights)
        sum_sq = math.fsum(w * w for w in weights)
        ess = (total * total) / sum_sq if sum_sq > 0.0 else 0.0

    return RakingResult(
        converged=converged,
        weights=weights,
        rounds=rounds,
        achieved=achieved,
        failure_reason=failure,
        effective_sample_size=ess,
    )
