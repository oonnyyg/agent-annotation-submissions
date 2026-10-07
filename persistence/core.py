from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass
from typing import Any


INFINITY_MARKER = "infinity"


class InputError(ValueError):
    """Raised when the points or queries do not match the required schema."""


@dataclass(frozen=True)
class Point:
    identifier: str
    x: float
    y: float


@dataclass(frozen=True)
class MergeRecord:
    distance: float
    surviving_component: str
    merged_components: tuple[str, ...]
    component_count_after: int

    def to_dict(self) -> dict[str, object]:
        return {
            "distance": self.distance,
            "surviving_component": self.surviving_component,
            "merged_components": list(self.merged_components),
            "component_count_after": self.component_count_after,
        }


@dataclass(frozen=True)
class Interval:
    identifier: str
    birth: float
    death: float | None

    def is_alive_at(self, scale: float) -> bool:
        if self.death is None:
            return True
        return scale < self.death

    def to_dict(self) -> dict[str, object]:
        return {
            "identifier": self.identifier,
            "birth": self.birth,
            "death": self.death if self.death is not None else INFINITY_MARKER,
        }


@dataclass(frozen=True)
class ScaleGroup:
    scale: float
    component_count: int
    groups: tuple[tuple[str, tuple[str, ...]], ...]

    def to_dict(self) -> dict[str, object]:
        return {
            "scale": self.scale,
            "component_count": self.component_count,
            "groups": [
                {"representative": representative, "members": list(members)}
                for representative, members in self.groups
            ],
        }


@dataclass(frozen=True)
class PersistenceResult:
    intervals: tuple[Interval, ...]
    merges: tuple[MergeRecord, ...]
    scale_groups: tuple[ScaleGroup, ...]

    def to_dict(self) -> dict[str, object]:
        return {
            "infinity_marker": INFINITY_MARKER,
            "interval_convention": "[birth, death)",
            "intervals": [interval.to_dict() for interval in self.intervals],
            "merges": [merge.to_dict() for merge in self.merges],
            "scales": [group.to_dict() for group in self.scale_groups],
        }


def load_points(csv_path: str) -> tuple[Point, ...]:
    try:
        with open(csv_path, newline="", encoding="utf-8-sig") as handle:
            reader = csv.DictReader(handle)
            if reader.fieldnames is None:
                raise InputError("CSV input must contain a header row")
            _require_columns(reader.fieldnames, {"id", "x", "y"})
            rows = list(reader)
    except OSError as exc:
        raise InputError(f"cannot read CSV input: {exc}") from exc

    if not 1 <= len(rows) <= 300:
        raise InputError("number of points must be between 1 and 300")

    points: list[Point] = []
    seen: set[str] = set()
    for line_number, row in enumerate(rows, start=2):
        identifier = (row.get("id") or "").strip()
        if not identifier:
            raise InputError(f"row {line_number}: id is required")
        if identifier in seen:
            raise InputError(f"duplicate point id: {identifier}")
        seen.add(identifier)
        try:
            x = float(row["x"])
            y = float(row["y"])
        except (TypeError, ValueError) as exc:
            raise InputError(f"row {line_number}: x and y must be numbers") from exc
        if not math.isfinite(x) or not math.isfinite(y):
            raise InputError(f"row {line_number}: x and y must be finite")
        points.append(Point(identifier, x, y))

    return tuple(points)


def load_scales(json_path: str) -> tuple[float, ...]:
    try:
        with open(json_path, encoding="utf-8") as handle:
            payload = json.load(handle)
    except (OSError, json.JSONDecodeError) as exc:
        raise InputError(f"cannot read query scales: {exc}") from exc
    return parse_scales(payload)


def parse_scales(payload: Any) -> tuple[float, ...]:
    if isinstance(payload, dict):
        raw_scales = payload.get("scales")
    else:
        raw_scales = payload
    if not isinstance(raw_scales, list) or not raw_scales:
        raise InputError("query scales must be a non-empty list")

    scales: list[float] = []
    for index, value in enumerate(raw_scales):
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise InputError(f"scale at index {index} must be a non-negative number")
        scale = float(value)
        if not math.isfinite(scale) or scale < 0:
            raise InputError(f"scale at index {index} must be finite and non-negative")
        scales.append(scale)
    return tuple(scales)


def analyze(points: tuple[Point, ...], query_scales: tuple[float, ...]) -> PersistenceResult:
    if not points:
        raise InputError("number of points must be between 1 and 300")

    ordered_points = tuple(sorted(points, key=lambda point: point.identifier))
    parent = {point.identifier: point.identifier for point in ordered_points}
    active = set(parent)
    intervals = {
        point.identifier: Interval(point.identifier, 0.0, None) for point in ordered_points
    }
    merges: list[MergeRecord] = []

    edge_batches = _edge_batches(ordered_points)
    for distance, edges in edge_batches:
        roots_before = {_find(parent, identifier) for identifier in active}
        links: dict[str, set[str]] = {root: set() for root in roots_before}
        for left, right in edges:
            left_root = _find(parent, left)
            right_root = _find(parent, right)
            if left_root == right_root:
                continue
            links.setdefault(left_root, set()).add(right_root)
            links.setdefault(right_root, set()).add(left_root)

        for component in _connected_components(links):
            if len(component) < 2:
                continue
            surviving = min(component)
            merged_components: list[str] = []
            for merged in component:
                parent[merged] = surviving
                if merged != surviving:
                    active.discard(merged)
                    merged_components.append(merged)
                    intervals[merged] = Interval(merged, 0.0, distance)
            merges.append(
                MergeRecord(
                    distance=distance,
                    surviving_component=surviving,
                    merged_components=tuple(sorted(merged_components)),
                    component_count_after=len(active),
                )
            )

    scale_groups = tuple(_groups_at_scale(ordered_points, parent, scale) for scale in query_scales)
    return PersistenceResult(
        intervals=tuple(intervals[point.identifier] for point in ordered_points),
        merges=tuple(merges),
        scale_groups=scale_groups,
    )


def _require_columns(actual: list[str], required: set[str]) -> None:
    missing = required.difference(column.strip() for column in actual)
    if missing:
        raise InputError(f"CSV input is missing columns: {', '.join(sorted(missing))}")


def _edge_batches(points: tuple[Point, ...]) -> tuple[tuple[float, tuple[tuple[str, str], ...]], ...]:
    by_distance: dict[float, list[tuple[str, str]]] = {}
    for index, left in enumerate(points):
        for right in points[index + 1 :]:
            distance = math.hypot(left.x - right.x, left.y - right.y)
            edge = tuple(sorted((left.identifier, right.identifier)))
            by_distance.setdefault(distance, []).append(edge)
    return tuple(
        (distance, tuple(sorted(by_distance[distance])))
        for distance in sorted(by_distance)
    )


def _find(parent: dict[str, str], identifier: str) -> str:
    root = identifier
    while parent[root] != root:
        root = parent[root]
    while parent[identifier] != identifier:
        grandparent = parent[identifier]
        parent[identifier] = root
        identifier = grandparent
    return root


def _connected_components(links: dict[str, set[str]]) -> tuple[frozenset[str], ...]:
    unseen = set(links)
    components: list[frozenset[str]] = []
    while unseen:
        start = min(unseen)
        stack = [start]
        unseen.remove(start)
        component: set[str] = set()
        while stack:
            node = stack.pop()
            component.add(node)
            for neighbor in links.get(node, ()):
                if neighbor in unseen:
                    unseen.remove(neighbor)
                    stack.append(neighbor)
        components.append(frozenset(component))
    return tuple(components)


def _groups_at_scale(
    points: tuple[Point, ...], final_parent: dict[str, str], scale: float
) -> ScaleGroup:
    parent = {point.identifier: point.identifier for point in points}
    for left, right in _edges_within(points, scale):
        _union(parent, left, right)

    grouped: dict[str, list[str]] = {}
    for point in points:
        grouped.setdefault(_find(parent, point.identifier), []).append(point.identifier)

    groups = tuple(
        (representative, tuple(members))
        for representative, members in sorted(grouped.items())
    )
    _ = final_parent
    return ScaleGroup(scale=scale, component_count=len(groups), groups=groups)


def _edges_within(
    points: tuple[Point, ...], scale: float
) -> tuple[tuple[str, str], ...]:
    edges: list[tuple[str, str]] = []
    for index, left in enumerate(points):
        for right in points[index + 1 :]:
            if math.hypot(left.x - right.x, left.y - right.y) <= scale:
                edges.append(tuple(sorted((left.identifier, right.identifier))))
    return tuple(edges)


def _union(parent: dict[str, str], left: str, right: str) -> None:
    left_root = _find(parent, left)
    right_root = _find(parent, right)
    if left_root != right_root:
        surviving = min(left_root, right_root)
        parent[left_root] = surviving
        parent[right_root] = surviving
