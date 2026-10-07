"""CSV/JSON loading and report writing."""

from __future__ import annotations

import csv
import json
import os

from .core import AnalysisResult, ScaleResult


class ConfigError(ValueError):
    """Raised for invalid analysis configuration."""


def load_csv(path: str) -> tuple[list[float], list[float]]:
    """Read a CSV with a header row of time,value columns."""
    times: list[float] = []
    values: list[float] = []
    with open(path, newline="", encoding="utf-8") as fh:
        reader = csv.reader(fh)
        header = next(reader, None)
        if header is None or len(header) < 2:
            raise ConfigError("CSV must have a header with at least two columns")
        for lineno, row in enumerate(reader, start=2):
            if not row or all(not cell.strip() for cell in row):
                continue
            if len(row) < 2:
                raise ConfigError(f"CSV line {lineno}: expected two columns")
            try:
                t = float(row[0])
                v = float(row[1])
            except ValueError as exc:
                raise ConfigError(f"CSV line {lineno}: {exc}") from exc
            times.append(t)
            values.append(v)
    if not times:
        raise ConfigError("CSV contains no data rows")
    return times, values


def load_config(path: str) -> dict:
    """Load and validate the JSON analysis configuration."""
    with open(path, encoding="utf-8") as fh:
        try:
            cfg = json.load(fh)
        except json.JSONDecodeError as exc:
            raise ConfigError(f"invalid JSON config: {exc}") from exc
    if not isinstance(cfg, dict):
        raise ConfigError("config must be a JSON object")
    tau0 = cfg.get("tau0")
    if not isinstance(tau0, (int, float)) or isinstance(tau0, bool) or tau0 <= 0:
        raise ConfigError("config 'tau0' must be a positive number")
    m_list = cfg.get("m")
    if (
        not isinstance(m_list, list)
        or not m_list
        or any(not isinstance(m, int) or isinstance(m, bool) or m < 1 for m in m_list)
    ):
        raise ConfigError("config 'm' must be a non-empty list of positive integers")
    detrend = cfg.get("detrend", False)
    if not isinstance(detrend, bool):
        raise ConfigError("config 'detrend' must be a boolean")
    return {"tau0": float(tau0), "m": m_list, "detrend": detrend}


def _scale_row(s: ScaleResult) -> dict:
    return {
        "m": s.m,
        "tau": s.tau,
        "windows": s.windows,
        "deviation": s.deviation,
        "variance": s.variance,
        "status": s.status,
    }


def result_to_dict(result: AnalysisResult) -> dict:
    doc = {
        "tau0": result.tau0,
        "detrended": result.detrended,
        "n_samples": result.n_samples,
        "segments": result.segments,
        "scales": [_scale_row(s) for s in result.scales],
    }
    if result.fit is not None:
        doc["fit"] = result.fit
    if result.raw_scales is not None:
        doc["raw_scales"] = [_scale_row(s) for s in result.raw_scales]
    return doc


def write_json_report(result: AnalysisResult, path: str) -> None:
    doc = result_to_dict(result)
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump(doc, fh, indent=2, allow_nan=False)
        fh.write("\n")
    os.replace(tmp, path)


def _fmt(x: float | None) -> str:
    return "" if x is None else repr(x)


def write_csv_report(result: AnalysisResult, path: str) -> None:
    tmp = path + ".tmp"
    with open(tmp, "w", newline="", encoding="utf-8") as fh:
        writer = csv.writer(fh)
        writer.writerow(["tau", "m", "windows", "deviation", "variance", "status"])
        for s in result.scales:
            writer.writerow(
                [repr(s.tau), s.m, s.windows, _fmt(s.deviation), _fmt(s.variance), s.status]
            )
    os.replace(tmp, path)
