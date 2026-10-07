"""Data structures, input loading/validation, and enrichment score computation."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass, field
from pathlib import Path


@dataclass(frozen=True)
class FeatureScore:
    """A single feature and its finite numeric score."""

    feature_id: str
    score: float


@dataclass
class FeatureSet:
    """A named collection of feature identifiers (order as given in input)."""

    name: str
    members: list[str]


@dataclass
class EnrichmentResult:
    """Result of running one feature set against one ranked list."""

    name: str
    status: str  # "ok" or "not_computable"
    reason: str = ""
    enrichment_score: float = 0.0
    peak_position: int = -1  # 0-based index into the ranked list
    member_count: int = 0
    ignored_members: list[str] = field(default_factory=list)
    hit_positions: list[int] = field(default_factory=list)
    p_value: float | None = None
    q_value: float | None = None
    running_sum: list[float] = field(default_factory=list)


def load_feature_scores(path: str | Path) -> list[FeatureScore]:
    """Load a CSV with columns ``feature_id,score`` and return the ranked list.

    Feature identifiers must be unique and scores must be finite numbers.
    The list is sorted by score descending; ties are broken by identifier.
    """
    records: list[FeatureScore] = []
    seen: set[str] = set()
    with open(path, newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        if reader.fieldnames is None or "feature_id" not in reader.fieldnames or "score" not in reader.fieldnames:
            raise ValueError("score CSV must have 'feature_id' and 'score' columns")
        for row_number, row in enumerate(reader, start=2):
            feature_id = (row["feature_id"] or "").strip()
            if not feature_id:
                raise ValueError(f"row {row_number}: empty feature_id")
            if feature_id in seen:
                raise ValueError(f"row {row_number}: duplicate feature_id {feature_id!r}")
            seen.add(feature_id)
            try:
                score = float(row["score"])
            except (TypeError, ValueError):
                raise ValueError(f"row {row_number}: non-numeric score {row['score']!r}") from None
            if not math.isfinite(score):
                raise ValueError(f"row {row_number}: non-finite score {row['score']!r}")
            records.append(FeatureScore(feature_id, score))
    if not records:
        raise ValueError("score CSV contains no features")
    records.sort(key=lambda r: (-r.score, r.feature_id))
    return records


def load_feature_sets(path: str | Path) -> list[FeatureSet]:
    """Load feature sets from JSON.

    Accepted shapes: ``{"sets": [{"name": ..., "members": [...]}, ...]}``
    or a plain mapping ``{"name": [members...]}``.
    """
    with open(path, encoding="utf-8") as handle:
        data = json.load(handle)
    sets: list[FeatureSet] = []
    if isinstance(data, dict) and "sets" in data:
        entries = data["sets"]
        for entry in entries:
            sets.append(FeatureSet(str(entry["name"]), [str(m) for m in entry["members"]]))
    elif isinstance(data, dict):
        for name, members in data.items():
            sets.append(FeatureSet(str(name), [str(m) for m in members]))
    else:
        raise ValueError("set JSON must be an object or contain a 'sets' list")
    if not sets:
        raise ValueError("set JSON contains no feature sets")
    return sets


def compute_running_sum(
    ranked: list[FeatureScore],
    hit_positions: set[int],
) -> list[float]:
    """Cumulative walk along the ranked list for the given hit positions.

    Hits increase the walk by the member's absolute score normalized within
    the set (equal shares when all hit weights are zero); misses decrease it
    by a constant amount so the walk ends at zero.
    """
    total = len(ranked)
    hits = len(hit_positions)
    weights = {pos: abs(ranked[pos].score) for pos in hit_positions}
    weight_sum = sum(weights.values())
    if weight_sum == 0.0:
        weights = {pos: 1.0 for pos in hit_positions}
        weight_sum = float(hits)
    miss_penalty = 1.0 / (total - hits)
    walk: list[float] = []
    value = 0.0
    for pos in range(total):
        if pos in hit_positions:
            value += weights[pos] / weight_sum
        else:
            value -= miss_penalty
        walk.append(value)
    return walk


def enrichment_score(walk: list[float]) -> tuple[float, int]:
    """Signed enrichment score: the walk value of maximal absolute magnitude.

    Ties in absolute value prefer the positive value, then the earliest index.
    Returns ``(score, position)`` with a 0-based position.
    """
    best_score = 0.0
    best_pos = -1
    best_abs = -1.0
    for pos, value in enumerate(walk):
        magnitude = abs(value)
        if magnitude > best_abs or (magnitude == best_abs and value > best_score):
            best_abs = magnitude
            best_score = value
            best_pos = pos
    return best_score, best_pos
