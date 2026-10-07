"""Input parsing and validation for routes (CSV) and settings (JSON)."""

from __future__ import annotations

import csv
import json
import math
from dataclasses import dataclass
from pathlib import Path

from .route import Waypoint
from .vincenty import Ellipsoid, WGS84


class InputError(Exception):
    """Raised for malformed input files or invalid parameters."""


@dataclass(frozen=True)
class Settings:
    ellipsoid: Ellipsoid
    step: float  # sampling interval in metres
    tol: float  # iteration convergence tolerance
    max_iter: int  # iteration count limit


def load_settings(path: Path) -> Settings:
    try:
        raw = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise InputError(f"settings file {path}: {exc}") from exc
    if not isinstance(raw, dict):
        raise InputError(f"settings file {path}: top level must be an object")

    ell_raw = raw.get("ellipsoid", {})
    if not isinstance(ell_raw, dict):
        raise InputError("settings: 'ellipsoid' must be an object")
    a = ell_raw.get("semi_major_axis", WGS84.a)
    f = ell_raw.get("flattening", WGS84.f)
    try:
        ellipsoid = Ellipsoid(a=float(a), f=float(f))
    except (TypeError, ValueError) as exc:
        raise InputError(f"settings: invalid ellipsoid: {exc}") from exc

    def _number(key: str, default: float) -> float:
        value = raw.get(key, default)
        if isinstance(value, bool) or not isinstance(value, (int, float)):
            raise InputError(f"settings: '{key}' must be a number")
        if not math.isfinite(value):
            raise InputError(f"settings: '{key}' must be finite")
        return float(value)

    step = _number("sample_interval", 0.0)
    if step <= 0.0:
        raise InputError("settings: 'sample_interval' must be positive")
    tol = _number("tolerance", 1e-12)
    if tol <= 0.0:
        raise InputError("settings: 'tolerance' must be positive")
    max_iter_raw = raw.get("max_iterations", 200)
    if (isinstance(max_iter_raw, bool) or not isinstance(max_iter_raw, int)
            or max_iter_raw < 1):
        raise InputError(
            "settings: 'max_iterations' must be a positive integer")
    return Settings(ellipsoid=ellipsoid, step=step, tol=tol,
                    max_iter=max_iter_raw)


def load_routes(path: Path) -> dict[str, list[Waypoint]]:
    """Read routes CSV with columns route_id,lat,lon (header required).

    Rows for the same route keep file order. Errors name the route and
    the 1-based data row.
    """
    routes: dict[str, list[Waypoint]] = {}
    try:
        with path.open(newline="", encoding="utf-8") as handle:
            reader = csv.DictReader(handle)
            if reader.fieldnames is None:
                raise InputError(f"routes file {path}: empty file")
            required = {"route_id", "lat", "lon"}
            missing = required - set(reader.fieldnames)
            if missing:
                raise InputError(
                    f"routes file {path}: missing columns {sorted(missing)}")
            for row_number, row in enumerate(reader, start=1):
                route_id = (row.get("route_id") or "").strip()
                if not route_id:
                    raise InputError(
                        f"routes file {path} row {row_number}: "
                        "empty route_id")
                values: list[float] = []
                for column in ("lat", "lon"):
                    text = (row.get(column) or "").strip()
                    try:
                        value = float(text)
                    except ValueError:
                        raise InputError(
                            f"routes file {path} row {row_number} "
                            f"(route '{route_id}'): invalid {column} "
                            f"{text!r}") from None
                    if not math.isfinite(value):
                        raise InputError(
                            f"routes file {path} row {row_number} "
                            f"(route '{route_id}'): {column} must be finite")
                    values.append(value)
                lat, lon = values
                if not -90.0 <= lat <= 90.0:
                    raise InputError(
                        f"routes file {path} row {row_number} "
                        f"(route '{route_id}'): latitude {lat} out of range")
                if abs(lat) == 90.0:
                    raise InputError(
                        f"routes file {path} row {row_number} "
                        f"(route '{route_id}'): pole waypoints unsupported")
                if not -180.0 <= lon < 180.0:
                    raise InputError(
                        f"routes file {path} row {row_number} "
                        f"(route '{route_id}'): longitude {lon} out of range")
                routes.setdefault(route_id, []).append(
                    Waypoint(lat=lat, lon=lon, row=row_number))
    except OSError as exc:
        raise InputError(f"routes file {path}: {exc}") from exc
    return routes
