"""Command-line interface for the binary meta-analysis tool."""

from __future__ import annotations

import argparse
import sys

from .data import DataError, load_studies
from .report import build_report, to_csv, to_json
from .stats import ExcludedStudy, compute_effect, leave_one_out, meta_analyze


def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(
        prog="binary-meta",
        description=(
            "Meta-analysis of binary outcomes (odds ratios). "
            "Statistical summaries only; no decision recommendations."
        ),
    )
    parser.add_argument("input", help="CSV file with study-level 2x2 counts")
    parser.add_argument(
        "--format",
        choices=("json", "csv"),
        default="json",
        help="report format (default: json)",
    )
    parser.add_argument("--output", help="write report to this file instead of stdout")
    return parser


def run(input_path: str) -> dict:
    studies = load_studies(input_path)
    effects = []
    excluded = []
    for study in studies:
        result = compute_effect(study)
        if isinstance(result, ExcludedStudy):
            excluded.append(result)
        else:
            effects.append(result)
    meta = meta_analyze(effects)
    loo = leave_one_out(effects)
    return build_report(effects, excluded, meta, loo)


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    try:
        report = run(args.input)
    except DataError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2
    except OSError as exc:
        print(f"error: cannot read input: {exc}", file=sys.stderr)
        return 2
    text = to_json(report) if args.format == "json" else to_csv(report)
    if args.output:
        with open(args.output, "w", encoding="utf-8", newline="") as handle:
            handle.write(text + "\n")
    else:
        print(text)
    return 0


if __name__ == "__main__":
    sys.exit(main())
