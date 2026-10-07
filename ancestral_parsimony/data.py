"""CSV trait data and transition cost model loading."""

from __future__ import annotations

import csv
import json

from .errors import InputError

UNKNOWN = "?"
MAX_STATES = 6


def load_traits(path, leaf_ids):
    """Load trait CSV. Returns (column names, {leaf_id: [values]}).

    Every leaf must appear exactly once; a missing row is an error, not an
    unknown observation. Unknown values are written as '?'.
    """
    with open(path, encoding="utf-8", newline="") as fh:
        rows = [row for row in csv.reader(fh) if row]
    if not rows:
        raise InputError("trait CSV is empty")
    header = rows[0]
    if len(header) < 2 or header[0].strip() != "id":
        raise InputError("trait CSV header must start with 'id' followed by trait columns")
    columns = [h.strip() for h in header[1:]]
    if any(not c for c in columns):
        raise InputError("trait column names must be non-empty")
    traits = {}
    for lineno, row in enumerate(rows[1:], start=2):
        if len(row) != len(header):
            raise InputError(f"row {lineno}: expected {len(header)} fields, got {len(row)}")
        try:
            node_id = int(row[0].strip())
        except ValueError:
            raise InputError(f"row {lineno}: invalid node id {row[0]!r}") from None
        if node_id in traits:
            raise InputError(f"row {lineno}: duplicate data row for node {node_id}")
        traits[node_id] = [v.strip() for v in row[1:]]
    leaf_set = set(leaf_ids)
    data_set = set(traits)
    if data_set != leaf_set:
        missing = sorted(leaf_set - data_set)
        extra = sorted(data_set - leaf_set)
        parts = []
        if missing:
            parts.append(f"missing rows for leaves {missing}")
        if extra:
            parts.append(f"rows for non-leaf/unknown nodes {extra}")
        raise InputError("trait rows must correspond one-to-one with leaves: " + "; ".join(parts))
    return columns, traits


def load_costs(path):
    """Load the shared state set and transition cost matrix."""
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    if not isinstance(data, dict) or "states" not in data or "matrix" not in data:
        raise InputError("cost JSON must contain 'states' and 'matrix'")
    states = data["states"]
    if (
        not isinstance(states, list)
        or not states
        or len(states) > MAX_STATES
        or any(not isinstance(s, str) or not s for s in states)
        or len(set(states)) != len(states)
    ):
        raise InputError(f"'states' must be 1..{MAX_STATES} unique non-empty strings")
    matrix = data["matrix"]
    n = len(states)
    if not isinstance(matrix, list) or len(matrix) != n:
        raise InputError(f"'matrix' must have exactly {n} rows")
    for i, row in enumerate(matrix):
        if not isinstance(row, list) or len(row) != n:
            raise InputError(f"matrix row {i} must have exactly {n} entries")
        for j, value in enumerate(row):
            if isinstance(value, bool) or not isinstance(value, (int, float)):
                raise InputError(f"matrix[{i}][{j}] must be a number")
            if value < 0:
                raise InputError(f"matrix[{i}][{j}] must be non-negative")
        if row[i] != 0:
            raise InputError(f"matrix diagonal must be zero (matrix[{i}][{i}]={row[i]})")
    return states, matrix


def validate_observations(traits, columns, states):
    allowed = set(states)
    for node_id, values in traits.items():
        for col, value in zip(columns, values):
            if value == UNKNOWN:
                continue
            if value not in allowed:
                raise InputError(
                    f"leaf {node_id} column {col!r}: state {value!r} not in {states}"
                )
