"""CSV/JSON input parsing and report writing."""

from __future__ import annotations

import csv
import json
from dataclasses import dataclass, field

from .solver import SatelliteObs

OBS_COLUMNS = ("epoch", "sat_id", "x", "y", "z", "pseudorange", "sigma")


@dataclass
class EpochInput:
    epoch: str
    sats: list[SatelliteObs] = field(default_factory=list)
    initial: tuple[float, float, float, float] | None = None


def load_observations(path: str) -> dict[str, EpochInput]:
    """Load per-epoch satellite observations from CSV."""
    epochs: dict[str, EpochInput] = {}
    with open(path, newline="", encoding="utf-8") as handle:
        reader = csv.DictReader(handle)
        missing = [c for c in OBS_COLUMNS if c not in (reader.fieldnames or [])]
        if missing:
            raise ValueError(f"observations CSV missing columns: {missing}")
        for line, row in enumerate(reader, start=2):
            try:
                epoch = row["epoch"].strip()
                sat = SatelliteObs(
                    sat_id=row["sat_id"].strip(),
                    x=float(row["x"]), y=float(row["y"]), z=float(row["z"]),
                    pseudorange=float(row["pseudorange"]),
                    sigma=float(row["sigma"]))
            except (ValueError, AttributeError) as exc:
                raise ValueError(f"observations CSV line {line}: {exc}") from exc
            epochs.setdefault(epoch, EpochInput(epoch=epoch)).sats.append(sat)
    if not epochs:
        raise ValueError("observations CSV contains no rows")
    return epochs


def load_initials(path: str) -> dict[str, tuple[float, float, float, float]]:
    """Load per-epoch initial states [x, y, z, clock_m] from JSON."""
    with open(path, encoding="utf-8") as handle:
        data = json.load(handle)
    if isinstance(data, dict) and "epochs" in data:
        data = data["epochs"]
    if not isinstance(data, dict):
        raise ValueError("initials JSON must map epoch -> [x, y, z, clock_m]")
    out: dict[str, tuple[float, float, float, float]] = {}
    for epoch, value in data.items():
        if not isinstance(value, (list, tuple)) or len(value) != 4:
            raise ValueError(
                f"initials JSON epoch {epoch!r}: expected [x, y, z, clock_m]")
        out[str(epoch)] = tuple(float(v) for v in value)
    return out


def epoch_report_dict(epoch: str, record: dict) -> dict:
    return {"epoch": epoch, **record}


def write_json_report(path: str, reports: list[dict]) -> None:
    with open(path, "w", encoding="utf-8") as handle:
        json.dump({"epochs": reports}, handle, indent=2)
        handle.write("\n")


def write_csv_report(path: str, reports: list[dict]) -> None:
    """Flat per-epoch summary CSV (residuals stay in the JSON report)."""
    columns = [
        "epoch", "status", "error", "x", "y", "z", "clock_m",
        "iterations", "weighted_residual_sum_squares",
        "sigma_x", "sigma_y", "sigma_z", "sigma_clock",
        "loo_excluded", "loo_ok", "loo_position_shift_m",
        "loo_clock_shift_m", "loo_extrapolated_residual_m", "loo_error",
    ]
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.DictWriter(handle, fieldnames=columns)
        writer.writeheader()
        for report in reports:
            base = {
                "epoch": report["epoch"],
                "status": report["status"],
                "error": report.get("error", ""),
            }
            if report["status"] != "ok":
                writer.writerow(base)
                continue
            sol = report["solution"]
            cov = report["covariance"]
            base.update({
                "x": sol["x"], "y": sol["y"], "z": sol["z"],
                "clock_m": sol["clock_m"],
                "iterations": report["iterations"],
                "weighted_residual_sum_squares":
                    report["weighted_residual_sum_squares"],
                "sigma_x": cov["sigma_x"], "sigma_y": cov["sigma_y"],
                "sigma_z": cov["sigma_z"], "sigma_clock": cov["sigma_clock"],
            })
            loo = report.get("leave_one_out") or []
            if not loo:
                writer.writerow(base)
                continue
            for entry in loo:
                row = dict(base)
                row.update({
                    "loo_excluded": entry["excluded_sat_id"],
                    "loo_ok": entry["ok"],
                    "loo_position_shift_m": entry.get("position_shift_m"),
                    "loo_clock_shift_m": entry.get("clock_shift_m"),
                    "loo_extrapolated_residual_m":
                        entry.get("extrapolated_residual_m"),
                    "loo_error": entry.get("error", ""),
                })
                writer.writerow(row)
