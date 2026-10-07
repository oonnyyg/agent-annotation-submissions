"""CSV / JSON input loading for the transport CLI."""
from __future__ import annotations

import csv
import json
from pathlib import Path

from .model import Problem, Site, TransportError


def load_distribution(path: str | Path, side: str) -> tuple[Site, ...]:
    """Load a CSV with columns id,x,y,mass (header row required)."""
    sites: list[Site] = []
    try:
        with open(path, newline="", encoding="utf-8") as fh:
            reader = csv.DictReader(fh)
            required = {"id", "x", "y", "mass"}
            if reader.fieldnames is None or not required <= set(reader.fieldnames):
                raise TransportError(
                    f"{side}: CSV must have columns id,x,y,mass; got {reader.fieldnames}"
                )
            for lineno, row in enumerate(reader, start=2):
                try:
                    site = Site(
                        site_id=row["id"].strip(),
                        x=int(row["x"]),
                        y=int(row["y"]),
                        mass=int(row["mass"]),
                    )
                except (TypeError, ValueError) as exc:
                    raise TransportError(
                        f"{side}: invalid row {lineno}: {row!r} ({exc})"
                    ) from exc
                sites.append(site)
    except OSError as exc:
        raise TransportError(f"{side}: cannot read {path}: {exc}") from exc
    return tuple(sites)


def load_config(path: str | Path) -> tuple[str, frozenset[tuple[str, str]]]:
    """Load JSON config: {"cost_metric": ..., "disabled": [[src, dst], ...]}."""
    try:
        with open(path, encoding="utf-8") as fh:
            data = json.load(fh)
    except OSError as exc:
        raise TransportError(f"config: cannot read {path}: {exc}") from exc
    except json.JSONDecodeError as exc:
        raise TransportError(f"config: invalid JSON in {path}: {exc}") from exc
    if not isinstance(data, dict):
        raise TransportError("config: top-level JSON value must be an object")
    metric = data.get("cost_metric", "manhattan")
    if not isinstance(metric, str):
        raise TransportError("config: cost_metric must be a string")
    disabled_raw = data.get("disabled", [])
    if not isinstance(disabled_raw, list):
        raise TransportError("config: disabled must be a list of [source, target] pairs")
    disabled: set[tuple[str, str]] = set()
    for entry in disabled_raw:
        if (
            not isinstance(entry, (list, tuple))
            or len(entry) != 2
            or not all(isinstance(v, str) for v in entry)
        ):
            raise TransportError(
                f"config: disabled entries must be [source, target] string pairs, got {entry!r}"
            )
        disabled.add((entry[0], entry[1]))
    return metric, frozenset(disabled)


def load_problem(sources_csv: str, targets_csv: str, config_json: str) -> Problem:
    metric, disabled = load_config(config_json)
    return Problem(
        sources=load_distribution(sources_csv, "sources"),
        targets=load_distribution(targets_csv, "targets"),
        cost_metric=metric,
        disabled=disabled,
    )
