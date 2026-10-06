"""Command line interface: solve epochs and write JSON/CSV reports."""

from __future__ import annotations

import argparse
import math
import sys

from .io import load_initials, load_observations, write_csv_report, write_json_report
from .loo import leave_one_out
from .solver import SolverError, solve_epoch


def process_epoch(epoch: str, sats, initial) -> dict:
    """Solve one epoch; failures are reported, never raised to other epochs."""
    try:
        sol = solve_epoch(sats, initial)
    except SolverError as exc:
        return {
            "epoch": epoch, "status": "failed",
            "error": f"{type(exc).__name__}: {exc}",
        }
    cov = sol.covariance
    report = {
        "epoch": epoch,
        "status": "ok",
        "solution": {
            "x": sol.x, "y": sol.y, "z": sol.z, "clock_m": sol.clock,
        },
        "iterations": sol.iterations,
        "residuals_m": sol.residuals,
        "weighted_residual_sum_squares": sol.weighted_residual_sum_squares,
        "covariance": {
            "matrix": cov,
            "sigma_x": math.sqrt(cov[0][0]),
            "sigma_y": math.sqrt(cov[1][1]),
            "sigma_z": math.sqrt(cov[2][2]),
            "sigma_clock": math.sqrt(cov[3][3]),
        },
    }
    if len(sats) >= 5:
        report["leave_one_out"] = [
            {
                "excluded_sat_id": r.excluded_sat_id,
                "ok": r.ok,
                "error": r.error,
                "position_shift_m": r.position_shift_m,
                "clock_shift_m": r.clock_shift_m,
                "extrapolated_residual_m": r.extrapolated_residual_m,
            }
            for r in leave_one_out(sats, initial, sol)
        ]
    return report


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="pseudorange-lab",
        description=(
            "Offline weighted least-squares receiver position/clock solver "
            "for teaching pseudorange data (no live navigation signals)."))
    parser.add_argument("observations",
                        help="CSV with epoch,sat_id,x,y,z,pseudorange,sigma")
    parser.add_argument("initials",
                        help="JSON mapping epoch -> [x, y, z, clock_m]")
    parser.add_argument("--json-report", default="report.json",
                        help="output JSON report path (default: report.json)")
    parser.add_argument("--csv-report", default="report.csv",
                        help="output CSV report path (default: report.csv)")
    return parser


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        epochs = load_observations(args.observations)
        initials = load_initials(args.initials)
    except (OSError, ValueError) as exc:
        print(f"input error: {exc}", file=sys.stderr)
        return 2

    reports = []
    failures = 0
    for epoch, entry in epochs.items():
        initial = initials.get(epoch)
        if initial is None:
            failures += 1
            reports.append({
                "epoch": epoch, "status": "failed",
                "error": "MissingInitialError: no initial state for epoch",
            })
            continue
        report = process_epoch(epoch, entry.sats, initial)
        if report["status"] != "ok":
            failures += 1
        reports.append(report)

    write_json_report(args.json_report, reports)
    write_csv_report(args.csv_report, reports)
    ok = len(reports) - failures
    print(f"solved {ok}/{len(reports)} epochs; "
          f"reports: {args.json_report}, {args.csv_report}")
    return 1 if failures else 0


if __name__ == "__main__":
    sys.exit(main())
