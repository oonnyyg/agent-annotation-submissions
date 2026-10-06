"""Command line interface for the rainflow fatigue audit."""
from __future__ import annotations

import argparse
import csv
import json
import sys

from .damage import SNModel, evaluate_cycles, summarize
from .rainflow import count_cycles
from .turning_points import extract_turning_points, load_csv


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="rainflow-audit",
        description="Rainflow cycle counting and fatigue damage audit.",
    )
    parser.add_argument("input_csv", help="stress history CSV (time,stress)")
    parser.add_argument("model_json", help="S-N model JSON")
    parser.add_argument("--cycles-out", help="write per-cycle CSV to this path")
    parser.add_argument("--summary-out", help="write JSON summary to this path")
    args = parser.parse_args(argv)

    try:
        points = load_csv(args.input_csv)
        model = SNModel.from_json(args.model_json)
    except (ValueError, OSError, KeyError, json.JSONDecodeError) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 2

    cycles = count_cycles(extract_turning_points(points))
    results = evaluate_cycles(cycles, model)
    summary = summarize(results)

    if args.cycles_out:
        _write_cycles_csv(args.cycles_out, results)
    text = json.dumps(summary, indent=2)
    if args.summary_out:
        with open(args.summary_out, "w", encoding="utf-8") as handle:
            handle.write(text + "\n")
    else:
        print(text)
    if summary["warning"]:
        print(f"warning: {summary['warning']}", file=sys.stderr)
    return 0


def _write_cycles_csv(path: str, results) -> None:
    fields = [
        "start_time", "end_time", "stress_range", "mean", "amplitude",
        "count", "equivalent_amplitude", "life", "damage", "evaluable", "note",
    ]
    with open(path, "w", newline="", encoding="utf-8") as handle:
        writer = csv.writer(handle)
        writer.writerow(fields)
        for r in results:
            writer.writerow([
                r.cycle.start_time, r.cycle.end_time, r.cycle.stress_range,
                r.cycle.mean, r.cycle.amplitude, r.cycle.count,
                r.equivalent_amplitude, r.life, r.damage, r.evaluable,
                r.note or "",
            ])


if __name__ == "__main__":
    raise SystemExit(main())
