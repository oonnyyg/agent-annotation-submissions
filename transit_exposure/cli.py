"""Command-line interface: JSON parameters + CSV exposures -> CSV + JSON."""

from __future__ import annotations

import argparse
import csv
import json
import math
import sys

from .core import (
    IntegrationError,
    LimbDarkening,
    TransitGeometry,
    exposure_averaged_flux,
    external_contact_interval,
    instantaneous_flux,
    total_stellar_flux,
)

_PARAM_KEYS = ("radius_ratio", "impact", "t_closest", "velocity", "u1", "u2")


def load_parameters(path: str) -> tuple[LimbDarkening, TransitGeometry, float]:
    with open(path, "r", encoding="utf-8") as fh:
        raw = json.load(fh)
    missing = [k for k in _PARAM_KEYS if k not in raw]
    if missing:
        raise ValueError(f"missing parameter(s): {', '.join(missing)}")
    values = {}
    for key in _PARAM_KEYS + ("tolerance",):
        if key in raw:
            value = float(raw[key])
            if not math.isfinite(value):
                raise ValueError(f"parameter {key!r} must be finite")
            values[key] = value
    tol = values.get("tolerance", 1e-8)
    if tol <= 0.0:
        raise ValueError("tolerance must be positive")
    ld = LimbDarkening(values["u1"], values["u2"])
    geom = TransitGeometry(
        values["radius_ratio"], values["impact"], values["t_closest"], values["velocity"]
    )
    return ld, geom, tol


def read_exposures(path: str) -> list[tuple[float, float]]:
    rows = []
    with open(path, "r", encoding="utf-8", newline="") as fh:
        for record in csv.DictReader(fh):
            t_center = float(record["t_center"])
            duration = float(record["duration"])
            if not (math.isfinite(t_center) and math.isfinite(duration)):
                raise ValueError(f"non-finite exposure row: {record}")
            if duration < 0.0:
                raise ValueError(f"negative duration in row: {record}")
            rows.append((t_center, duration))
    return rows


def run(params_path: str, exposures_path: str, out_csv: str, out_json: str) -> dict:
    ld, geom, tol = load_parameters(params_path)
    exposures = read_exposures(exposures_path)

    rows = []
    converged = True
    errors = []
    for t_center, duration in exposures:
        record = {"t_center": t_center, "duration": duration}
        try:
            record["instantaneous"] = instantaneous_flux(ld, geom, t_center, tol * 0.1)
            record["exposure_mean"] = exposure_averaged_flux(
                ld, geom, t_center, duration, tol
            )
        except IntegrationError as exc:
            converged = False
            errors.append(str(exc))
            record["instantaneous"] = None
            record["exposure_mean"] = None
        rows.append(record)

    with open(out_csv, "w", encoding="utf-8", newline="") as fh:
        writer = csv.DictWriter(
            fh, fieldnames=["t_center", "duration", "instantaneous", "exposure_mean"]
        )
        writer.writeheader()
        writer.writerows(rows)

    contact = external_contact_interval(geom)
    diagnostics = {
        "converged": converged,
        "tolerance": tol,
        "total_stellar_flux": total_stellar_flux(ld),
        "external_contact_interval": list(contact) if contact else None,
        "n_exposures": len(rows),
        "errors": errors,
    }
    with open(out_json, "w", encoding="utf-8") as fh:
        json.dump(diagnostics, fh, indent=2)
        fh.write("\n")
    return diagnostics


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="transit-exposure",
        description="Limb-darkened transit light curves with finite-exposure averaging.",
    )
    parser.add_argument("parameters", help="JSON file with model parameters")
    parser.add_argument("exposures", help="CSV with t_center,duration columns")
    parser.add_argument("--out-csv", default="lightcurve.csv")
    parser.add_argument("--out-json", default="diagnostics.json")
    args = parser.parse_args(argv)
    try:
        diagnostics = run(args.parameters, args.exposures, args.out_csv, args.out_json)
    except (ValueError, OSError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    if not diagnostics["converged"]:
        print("warning: some exposures did not converge", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
