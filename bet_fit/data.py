"""CSV / JSON loading and validation for BET analysis."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass
from pathlib import Path


class DataError(ValueError):
    """Raised when input data or configuration is invalid."""


@dataclass(frozen=True)
class Sample:
    name: str
    pressures: tuple[float, ...]       # relative pressure p/p0, 0 < p < 1
    quantities: tuple[float, ...]      # adsorbed amount, mol/g


@dataclass(frozen=True)
class Config:
    cross_section_nm2: float           # adsorbate molecular cross-section, nm^2
    min_points: int = 4                # minimum points per fitted interval
    interval: tuple[float, float] | None = None  # manual closed interval, optional


def load_config(path: str | Path) -> Config:
    try:
        raw = json.loads(Path(path).read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise DataError(f"cannot read config {path}: {exc}") from exc
    if not isinstance(raw, dict):
        raise DataError("config must be a JSON object")
    try:
        sigma = float(raw["cross_section_nm2"])
    except (KeyError, TypeError, ValueError) as exc:
        raise DataError("config requires numeric 'cross_section_nm2'") from exc
    if not math.isfinite(sigma) or sigma <= 0:
        raise DataError("cross_section_nm2 must be a positive finite number")
    min_points = int(raw.get("min_points", 4))
    if min_points < 4:
        raise DataError("min_points must be at least 4")
    interval = raw.get("interval")
    if interval is not None:
        if (not isinstance(interval, list) or len(interval) != 2
                or not all(isinstance(v, (int, float)) for v in interval)):
            raise DataError("interval must be [lower, upper] relative pressures")
        lo, hi = float(interval[0]), float(interval[1])
        if not (0.0 < lo < hi < 1.0):
            raise DataError("interval must satisfy 0 < lower < upper < 1")
        interval = (lo, hi)
    return Config(cross_section_nm2=sigma, min_points=min_points, interval=interval)


def load_samples(path: str | Path) -> list[Sample]:
    """Load a CSV with columns: sample, relative_pressure, quantity_mol_per_g."""
    rows: dict[str, list[tuple[int, float, float]]] = {}
    try:
        with open(path, newline="", encoding="utf-8-sig") as fh:
            reader = csv.DictReader(fh)
            required = {"sample", "relative_pressure", "quantity_mol_per_g"}
            if reader.fieldnames is None or not required <= set(reader.fieldnames):
                raise DataError(
                    f"CSV must have columns: {', '.join(sorted(required))}")
            for lineno, row in enumerate(reader, start=2):
                name = (row["sample"] or "").strip()
                if not name:
                    raise DataError(f"line {lineno}: empty sample name")
                try:
                    p = float(row["relative_pressure"])
                    q = float(row["quantity_mol_per_g"])
                except (TypeError, ValueError) as exc:
                    raise DataError(
                        f"line {lineno}: non-numeric pressure or quantity") from exc
                if not math.isfinite(p) or not math.isfinite(q):
                    raise DataError(
                        f"line {lineno}: non-finite value "
                        f"(p={row['relative_pressure']}, q={row['quantity_mol_per_g']})")
                if not 0.0 < p < 1.0:
                    raise DataError(
                        f"line {lineno}: relative pressure {p} outside (0, 1)")
                if q <= 0.0:
                    raise DataError(
                        f"line {lineno}: quantity {q} must be positive (mol/g)")
                rows.setdefault(name, []).append((lineno, p, q))
    except OSError as exc:
        raise DataError(f"cannot read CSV {path}: {exc}") from exc
    if not rows:
        raise DataError("CSV contains no data rows")

    samples = []
    for name, entries in rows.items():
        entries.sort(key=lambda e: e[1])
        for (prev_line, prev_p, _), (line, p, _) in zip(entries, entries[1:]):
            if p == prev_p:
                raise DataError(
                    f"sample {name!r}: duplicate relative pressure {p} "
                    f"at lines {prev_line} and {line}")
        samples.append(Sample(
            name=name,
            pressures=tuple(p for _, p, _ in entries),
            quantities=tuple(q for _, _, q in entries),
        ))
    return samples
