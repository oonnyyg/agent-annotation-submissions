"""CSV/JSON input and report output for the gapped Allan CLI."""

from __future__ import annotations

import csv
import json
import math
import os
import tempfile
from dataclasses import dataclass

from .core import AnalysisError, TauResult


@dataclass
class Config:
    tau0: float
    m_values: list[int]
    detrend: bool = False
    time_column: str = "time"
    value_column: str = "frac_freq"


def load_config(path: str) -> Config:
    """Load and validate the JSON analysis configuration."""
    try:
        with open(path, "r", encoding="utf-8") as fh:
            raw = json.load(fh)
    except (OSError, json.JSONDecodeError) as exc:
        raise AnalysisError(f"cannot read config {path!r}: {exc}") from exc
    if not isinstance(raw, dict):
        raise AnalysisError("config must be a JSON object")
    try:
        tau0 = float(raw["tau0"])
    except (KeyError, TypeError, ValueError) as exc:
        raise AnalysisError("config requires a numeric 'tau0'") from exc
    if not math.isfinite(tau0) or tau0 <= 0:
        raise AnalysisError("config 'tau0' must be positive and finite")
    m_values = raw.get("m_values")
    if not isinstance(m_values, list) or not m_values:
        raise AnalysisError("config requires a non-empty 'm_values' list")
    for m in m_values:
        if not isinstance(m, int) or isinstance(m, bool) or m < 1:
            raise AnalysisError(f"invalid averaging length in config: {m!r}")
    detrend = bool(raw.get("detrend", False))
    time_column = str(raw.get("time_column", "time"))
    value_column = str(raw.get("value_column", "frac_freq"))
    return Config(
        tau0=tau0,
        m_values=sorted(set(m_values)),
        detrend=detrend,
        time_column=time_column,
        value_column=value_column,
    )


def read_csv(path: str, cfg: Config) -> tuple[list[float], list[float]]:
    """Read timestamps and dimensionless fractional frequencies."""
    times: list[float] = []
    values: list[float] = []
    try:
        with open(path, "r", encoding="utf-8", newline="") as fh:
            reader = csv.DictReader(fh)
            if reader.fieldnames is None:
                raise AnalysisError(f"CSV file {path!r} has no header row")
            for col in (cfg.time_column, cfg.value_column):
                if col not in reader.fieldnames:
                    raise AnalysisError(
                        f"CSV file {path!r} is missing column {col!r}"
                    )
            for lineno, row in enumerate(reader, start=2):
                try:
                    t = float(row[cfg.time_column])
                    y = float(row[cfg.value_column])
                except (TypeError, ValueError) as exc:
                    raise AnalysisError(
                        f"non-numeric value at {path!r} line {lineno}"
                    ) from exc
                if not (math.isfinite(t) and math.isfinite(y)):
                    raise AnalysisError(
                        f"non-finite value at {path!r} line {lineno}"
                    )
                times.append(t)
                values.append(y)
    except OSError as exc:
        raise AnalysisError(f"cannot read CSV {path!r}: {exc}") from exc
    return times, values


def _atomic_write(path: str, text: str) -> None:
    """Write text atomically so failures never clobber existing files."""
    directory = os.path.dirname(os.path.abspath(path))
    fd, tmp = tempfile.mkstemp(dir=directory, prefix=".tmp-")
    try:
        with os.fdopen(fd, "w", encoding="utf-8", newline="") as fh:
            fh.write(text)
        os.replace(tmp, path)
    except BaseException:
        try:
            os.unlink(tmp)
        except OSError:
            pass
        raise


def results_to_csv(results: list[TauResult]) -> str:
    """Render tau-sorted results as CSV text."""
    import io as _io

    buf = _io.StringIO()
    writer = csv.writer(buf)
    writer.writerow(["m", "tau_s", "allan_dev", "allan_var", "pairs", "status"])
    for r in sorted(results, key=lambda r: r.tau):
        writer.writerow([
            r.m,
            f"{r.tau:.12g}",
            "" if r.adev is None else f"{r.adev:.12g}",
            "" if r.avar is None else f"{r.avar:.12g}",
            r.pairs,
            r.status,
        ])
    return buf.getvalue()


def write_reports(
    csv_path: str,
    json_path: str,
    report: dict,
    results: list[TauResult],
) -> None:
    """Write CSV and JSON reports only after full success."""
    _atomic_write(csv_path, results_to_csv(results))
    _atomic_write(
        json_path, json.dumps(report, indent=2, ensure_ascii=False) + "\n"
    )
