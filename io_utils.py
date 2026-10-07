"""CSV/JSON loading, validation, and report rendering."""

from __future__ import annotations

import csv
import io
import json
import math

from persistence import INFINITY, MAX_POINTS, PersistenceResult


def load_points(path: str) -> dict[str, tuple[float, float]]:
    points: dict[str, tuple[float, float]] = {}
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.reader(fh)
        for lineno, row in enumerate(reader, start=1):
            if lineno == 1 and row and row[0].strip().lower() == "id":
                continue  # optional header row
            if not row or all(not cell.strip() for cell in row):
                continue
            if len(row) != 3:
                raise ValueError(f"line {lineno}: expected id,x,y")
            pid = row[0].strip()
            if not pid:
                raise ValueError(f"line {lineno}: empty id")
            if pid in points:
                raise ValueError(f"line {lineno}: duplicate id {pid!r}")
            try:
                x, y = float(row[1]), float(row[2])
            except ValueError:
                raise ValueError(f"line {lineno}: non-numeric coordinate") from None
            if not (math.isfinite(x) and math.isfinite(y)):
                raise ValueError(f"line {lineno}: non-finite coordinate")
            points[pid] = (x, y)
    if not 1 <= len(points) <= MAX_POINTS:
        raise ValueError(f"point count must be 1..{MAX_POINTS}, got {len(points)}")
    return points


def load_thresholds(path: str) -> list[float]:
    with open(path, encoding="utf-8") as fh:
        data = json.load(fh)
    if isinstance(data, dict):
        data = data.get("thresholds")
    if not isinstance(data, list) or not data:
        raise ValueError("JSON must be a non-empty list or {'thresholds': [...]}")
    thresholds = []
    for value in data:
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise ValueError(f"invalid threshold {value!r}")
        value = float(value)
        if not math.isfinite(value) or value < 0:
            raise ValueError(f"threshold must be a non-negative finite number: {value!r}")
        thresholds.append(value)
    return thresholds


def build_report(result: PersistenceResult, thresholds: list[float]) -> dict:
    intervals = [
        {"id": pid, "birth": birth, "death": death}
        for pid, birth, death in result.intervals()
    ]
    queries = []
    for t in thresholds:
        groups = result.components_at(t)
        alive = sum(
            1 for _, _, death in result.intervals()
            if death == INFINITY or death > t
        )
        assert alive == len(groups), "interval count disagrees with grouping"
        queries.append({
            "threshold": t,
            "component_count": len(groups),
            "components": groups,
        })
    return {
        "points": {pid: list(xy) for pid, xy in sorted(result.points.items())},
        "intervals": intervals,
        "merges": [
            {"distance": m.distance, "died": m.died, "survivor": m.survivor}
            for m in result.merges
        ],
        "queries": queries,
    }


def intervals_csv(report: dict) -> str:
    buf = io.StringIO()
    writer = csv.writer(buf)
    writer.writerow(["id", "birth", "death"])
    for row in report["intervals"]:
        writer.writerow([row["id"], row["birth"], row["death"]])
    return buf.getvalue()


def merges_csv(report: dict) -> str:
    buf = io.StringIO()
    writer = csv.writer(buf)
    writer.writerow(["distance", "died", "survivor"])
    for row in report["merges"]:
        writer.writerow([row["distance"], row["died"], row["survivor"]])
    return buf.getvalue()
