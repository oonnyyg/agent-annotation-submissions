"""Input loading and validation for the Green-Ampt calculator."""

from __future__ import annotations

import csv
import json
from pathlib import Path

from .model import Segment, SoilParams

REQUIRED_SOIL_KEYS = ("ks", "psi", "delta_theta")


def load_soil(path: str | Path) -> SoilParams:
    """Load soil parameters from a JSON file.

    Expected keys: ks (mm/h), psi (mm), delta_theta (-). All must be
    positive and delta_theta must not exceed one.
    """
    with open(path, "r", encoding="utf-8") as fh:
        data = json.load(fh)
    missing = [k for k in REQUIRED_SOIL_KEYS if k not in data]
    if missing:
        raise ValueError(f"soil file missing keys: {', '.join(missing)}")
    try:
        return SoilParams(
            ks=float(data["ks"]),
            psi=float(data["psi"]),
            delta_theta=float(data["delta_theta"]),
        )
    except (TypeError, ValueError) as exc:
        raise ValueError(f"invalid soil parameters: {exc}") from exc


def load_segments(path: str | Path) -> list[Segment]:
    """Load rainfall segments from CSV with columns start,end,rate.

    Times in hours, rates in mm/h. Rows must be contiguous and ordered:
    overlaps, gaps, negative rates and non-positive durations are errors.
    """
    segments: list[Segment] = []
    with open(path, "r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        required = {"start", "end", "rate"}
        if reader.fieldnames is None or not required <= set(reader.fieldnames):
            raise ValueError("CSV must have columns: start,end,rate")
        for lineno, row in enumerate(reader, start=2):
            try:
                seg = Segment(
                    start=float(row["start"]),
                    end=float(row["end"]),
                    rate=float(row["rate"]),
                )
            except (TypeError, ValueError) as exc:
                raise ValueError(f"CSV line {lineno}: {exc}") from exc
            if seg.end <= seg.start:
                raise ValueError(f"CSV line {lineno}: end must exceed start")
            if seg.rate < 0.0:
                raise ValueError(f"CSV line {lineno}: negative rainfall rate")
            segments.append(seg)

    if not segments:
        raise ValueError("CSV contains no segments")
    for prev, cur in zip(segments, segments[1:]):
        if cur.start < prev.end:
            raise ValueError(
                f"segments overlap: [{prev.start}, {prev.end}) and "
                f"[{cur.start}, {cur.end})"
            )
        if cur.start > prev.end:
            raise ValueError(
                f"gap between segments: {prev.end} -> {cur.start}"
            )
    return segments