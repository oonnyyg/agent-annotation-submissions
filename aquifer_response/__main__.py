"""Command-line entry point: python -m aquifer_response job.json -o outdir"""

from __future__ import annotations

import argparse
import json
import os
import sys

from .config_io import InputError, parse_job
from .fitting import FittingError, score_candidates
from .model import EvaluationStats, ModelError, simulate, validate_geometry
from .report import build_report, write_csv, write_report
from .wellfunction import WellFunctionError


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="python -m aquifer_response",
        description="Multi-well pumping response in an ideal confined aquifer.",
    )
    parser.add_argument("job", help="path to the JSON job file")
    parser.add_argument(
        "-o",
        "--output-dir",
        default=".",
        help="directory for drawdown.csv and report.json (default: cwd)",
    )
    parser.add_argument("--csv-name", default="drawdown.csv")
    parser.add_argument("--report-name", default="report.json")
    args = parser.parse_args(argv)

    try:
        with open(args.job, encoding="utf-8") as fh:
            data = json.load(fh)
        job = parse_job(data)

        validate_geometry(job["wells"], job["points"])

        stats = EvaluationStats()
        series = simulate(
            job["aquifer"],
            job["wells"],
            job["points"],
            job["times"],
            job["config"],
            stats,
        )

        ranked = None
        if job["candidates"]:
            if not job["observations"]:
                raise InputError("candidates require observations to score against")
            ranked = score_candidates(
                job["candidates"],
                job["observations"],
                job["wells"],
                job["points"],
                job["config"],
                stats,
            )

        report = build_report(
            job["aquifer"],
            job["wells"],
            job["points"],
            job["times"],
            series,
            job["observations"],
            ranked,
            stats,
        )

        os.makedirs(args.output_dir, exist_ok=True)
        csv_path = os.path.join(args.output_dir, args.csv_name)
        report_path = os.path.join(args.output_dir, args.report_name)
        write_csv(csv_path, job["times"], job["wells"], job["points"], series)
        write_report(report_path, report)
    except (InputError, ModelError, FittingError, WellFunctionError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    except OSError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    print(f"wrote {csv_path}")
    print(f"wrote {report_path}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
